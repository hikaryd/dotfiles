"""Experimental Seatbelt primitive. Not an audited production security boundary."""
import argparse
import errno
import fcntl
import ipaddress
import os
import pwd
import re
import stat
import termios
import urllib.parse

SANDBOX = '/usr/bin/sandbox-exec'


def validate_proxy(url):
    if not isinstance(url, str) or re.search(r'[\x00-\x20\x7f]', url):
        raise ValueError('invalid proxy URL')
    if re.search(r'%(?![0-9a-fA-F]{2})', url) or re.search(r'%(?:0[0-9a-f]|1[0-9a-f]|7f)', url, re.I):
        raise ValueError('invalid or control percent escape')
    if '\\' in url or any(ord(c) > 126 for c in url):
        raise ValueError('proxy URL must use ASCII URL encoding')
    try:
        parsed = urllib.parse.urlsplit(url)
        if parsed.scheme not in ('http', 'https') or not parsed.netloc or parsed.path or parsed.query or parsed.fragment:
            raise ValueError('proxy must be an http(s) authority only')
        if '?' in url or '#' in url or parsed.hostname is None or parsed.port is None:
            raise ValueError('proxy requires an explicit port')
        if not 1 <= parsed.port <= 65535 or ':' in parsed.hostname:
            raise ValueError('invalid proxy address or port')
        ipaddress.IPv4Address(parsed.hostname)
        if '%' in parsed.netloc.rsplit('@', 1)[-1] or parsed.netloc.count('@') > 1:
            raise ValueError('invalid authority')
        return parsed
    except (ValueError, UnicodeError) as exc:
        raise ValueError('proxy_url must be http(s)://[user:password@]NUMERIC_IPV4:PORT without a path') from None


def _quote(value):
    if any(ord(c) < 32 or ord(c) == 127 for c in value):
        raise ValueError('control character in path')
    return '"' + value.replace('\\', '\\\\').replace('"', '\\"') + '"'


def _directory(path):
    path = os.path.abspath(os.fspath(path))
    real = os.path.realpath(path)
    if path != real or not os.path.isdir(real):
        raise ValueError('root must be an existing canonical directory: ' + path)
    forbidden = {'/', '/Users', os.path.realpath(pwd.getpwuid(os.getuid()).pw_dir), '/Applications', '/System', '/Library', '/usr', '/opt', '/private', '/private/var', '/private/tmp', '/var', '/tmp'}
    if real in forbidden:
        raise ValueError('broad root forbidden: ' + path)
    return real


def validate_stdio():
    """Pipes and the caller's terminal are intentional host input/output capabilities."""
    terminals = set()
    for fd in range(3):
        try:
            info = os.fstat(fd)
        except OSError:
            raise ValueError('standard descriptors must be open') from None
        if stat.S_ISFIFO(info.st_mode) and info.st_nlink == 0:
            continue
        if stat.S_ISREG(info.st_mode) and fcntl.fcntl(fd, fcntl.F_GETFL) & os.O_ACCMODE == os.O_RDONLY:
            continue
        if stat.S_ISCHR(info.st_mode):
            if info.st_rdev == os.stat('/dev/null').st_rdev:
                continue
            if os.isatty(fd):
                path = os.path.realpath(os.ttyname(fd))
                if re.fullmatch(r'/dev/ttys[0-9]+', path):
                    terminals.add(path)
                    continue
        raise ValueError('unsafe standard descriptor: use a local terminal or anonymous pipes, not sockets, writable files or devices')
    return sorted(terminals)


def close_extra_fds():
    # Existing descriptors may exceed a subsequently lowered RLIMIT_NOFILE.
    for name in os.listdir('/dev/fd'):
        fd = int(name)
        if fd >= 3:
            try:
                os.close(fd)
            except OSError as error:
                if error.errno != errno.EBADF:
                    raise


def make_profile(relay_port, write_roots, protected_paths, terminals=()):
    """Permit TCP to localhost on relay_port. Both loopback families must be trusted."""
    if type(relay_port) is not int or not 1 <= relay_port <= 65535:
        raise ValueError('relay port must be an integer from 1 to 65535')
    roots = [_directory(p) for p in write_roots]
    if not roots:
        raise ValueError('at least one write root is required')
    protected = []
    for p in protected_paths:
        p = os.path.abspath(os.fspath(p))
        if not os.path.exists(p) or os.path.realpath(p) != p:
            raise ValueError('protected path must exist and be canonical: ' + p)
        protected.append(p)
    protected.append(os.path.realpath(__file__))
    lines = ['(version 1)', '(deny default)', '(allow process-fork process-exec)',
             '(allow signal process-info* (target same-sandbox))',
             '(allow file-read* (require-not (regex #"^/dev/(tty|pty)")))', '(allow sysctl-read)',
             '(allow file-write* (literal "/dev/null") (literal "/dev/zero"))',
             '(deny file-ioctl (ioctl-command {}))'.format(termios.TIOCSTI),
             '(allow network-outbound (remote tcp {}))'.format(_quote('localhost:{}'.format(relay_port)))]
    for terminal in terminals:
        if not re.fullmatch(r'/dev/ttys[0-9]+', terminal):
            raise ValueError('invalid inherited terminal')
        lines.append('(allow file-read* file-write* file-ioctl (literal {}))'.format(_quote(terminal)))
    if terminals:
        lines.append('(allow file-read* file-write* file-ioctl (literal "/dev/tty"))')
    for root in roots:
        lines.append('(allow file-write* (subpath {}))'.format(_quote(root)))
    # Deny removal of every ancestor as well as the protected subtree.
    for path in protected:
        lines.append('(deny file-write* (subpath {}))'.format(_quote(path)))
        parent = os.path.dirname(path)
        while parent != '/':
            lines.append('(deny file-write-unlink (literal {}))'.format(_quote(parent)))
            parent = os.path.dirname(parent)
    home = os.path.realpath(pwd.getpwuid(os.getuid()).pw_dir)
    for path in ('Library/LaunchAgents', '.ssh', '.config', '.zshrc', '.zprofile', '.bashrc', '.bash_profile', '.profile', '.gitconfig'):
        lines.append('(deny file-write* (subpath {}))'.format(_quote(os.path.join(home, path))))
    for path in ('/Network', '/Volumes', os.path.join(home, 'Library/CloudStorage')):
        lines.append('(deny file-read* (subpath {}))'.format(_quote(os.path.realpath(path))))
    return '\n'.join(lines) + '\n'


def sandbox_argv(profile, command):
    if not command:
        raise ValueError('command required')
    return [SANDBOX, '-p', profile, '--'] + list(command)


def proxy_env(url, home, tmp):
    validate_proxy(url)
    home, tmp = _directory(home), _directory(tmp)
    env = {'PATH': '/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin'}
    for key in ('TERM', 'COLORTERM', 'LANG', 'LC_ALL', 'LC_CTYPE'):
        if key in os.environ:
            env[key] = os.environ[key]
    env.update({'HOME': home, 'TMPDIR': tmp, 'http_proxy': url, 'https_proxy': url, 'HTTP_PROXY': url, 'HTTPS_PROXY': url, 'ALL_PROXY': url, 'all_proxy': url})
    return env


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--proxy', required=True)
    parser.add_argument('--home', required=True)
    parser.add_argument('--tmp', required=True)
    parser.add_argument('--write-root', action='append', default=[])
    parser.add_argument('--protect', action='append', default=[])
    parser.add_argument('command', nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command:
        parser.error('command required')
    parsed = validate_proxy(args.proxy)
    if parsed.scheme != 'http' or parsed.hostname != '127.0.0.1':
        parser.error('proxy must be the trusted local IPv4 HTTP relay')
    profile = make_profile(parsed.port, args.write_root + [args.home, args.tmp], args.protect, validate_stdio())
    env = proxy_env(args.proxy, args.home, args.tmp)
    argv = sandbox_argv(profile, command)
    close_extra_fds()
    os.execve(SANDBOX, argv, env)


if __name__ == '__main__':
    main()
