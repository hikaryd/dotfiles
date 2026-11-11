import ctypes
import os
import pty
import shutil
import socket
import subprocess
import sys
import tempfile
import unittest
from unittest import mock
import guard

HERE = os.path.realpath(os.path.dirname(__file__))
PYTHON = os.path.realpath(sys.executable)


class GuardTests(unittest.TestCase):
    @unittest.skipUnless(sys.platform == 'darwin', 'requires macOS descriptors')
    def test_descriptors_above_lowered_limit_are_closed(self):
        with tempfile.TemporaryDirectory(dir=HERE) as root:
            root = os.path.realpath(root)
            setup = '''import os,resource,sys
fd=os.open('/dev/null',os.O_RDONLY)
os.dup2(fd,256,inheritable=True)
os.close(fd)
os.fstat(256)
soft,hard=resource.getrlimit(resource.RLIMIT_NOFILE)
resource.setrlimit(resource.RLIMIT_NOFILE,(64,hard))
'''
            check = 'import os,sys\ntry: os.fstat(256)\nexcept OSError: sys.exit(0)\nsys.exit(9)'
            command = [PYTHON, os.path.join(HERE, 'guard.py'), '--proxy', 'http://127.0.0.1:1',
                       '--home', root, '--tmp', root, '--', PYTHON, '-c', check]
            for suffix in ('os.execv(sys.executable,' + repr(command) + ')',
                           'sys.path.insert(0,' + repr(HERE) + ');import launcher;sys.argv=["launcher","claude-setup","settings"];launcher.main()\n' + check):
                result = subprocess.run([PYTHON, '-c', setup + suffix], stdin=subprocess.DEVNULL,
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    @unittest.skipUnless(sys.platform == 'darwin', 'requires macOS sandbox')
    def test_stdio_and_terminal_boundary(self):
        with tempfile.TemporaryDirectory(dir=HERE) as root:
            root = os.path.realpath(root)
            cli = [PYTHON, os.path.join(HERE, 'guard.py'), '--proxy', 'http://127.0.0.1:1',
                   '--home', root, '--tmp', root, '--', PYTHON, '-c', 'import os;os.write(1,b"ran")']
            result = subprocess.run(cli, stdin=subprocess.DEVNULL, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout, b'ran')
            left, right = socket.socketpair()
            try:
                result = subprocess.run(cli, stdin=left, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b'unsafe standard descriptor', result.stderr)
            finally:
                left.close()
                right.close()
            with open(os.path.join(root, 'output'), 'w+b') as output:
                result = subprocess.run(cli, stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.PIPE)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(output.tell(), 0)
            master, slave = pty.openpty()
            other_master, other_slave = pty.openpty()
            try:
                own, foreign = os.ttyname(slave), os.ttyname(other_slave)
                code = '''import os,sys,termios,fcntl
fd=os.open(sys.argv[1],os.O_RDWR|os.O_NOCTTY)
termios.tcsetattr(fd,termios.TCSANOW,termios.tcgetattr(fd))
os.write(fd,b'own-terminal-ok')
try: fcntl.ioctl(fd,termios.TIOCSTI,b'x')
except OSError: pass
else: raise AssertionError('input injection allowed')
for flags in (os.O_RDONLY,os.O_WRONLY,os.O_RDWR):
 try: os.open(sys.argv[2],flags|os.O_NOCTTY)
 except OSError: pass
 else: raise AssertionError('foreign terminal opened')
'''
                result = subprocess.run(cli[:-3] + [PYTHON, '-c', code, own, foreign],
                                        stdin=slave, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(b'own-terminal-ok', os.read(master, 1024))
                fd = os.open(foreign, os.O_RDWR | os.O_NOCTTY)
                os.close(fd)
            finally:
                for fd in (master, slave, other_master, other_slave):
                    os.close(fd)

    def test_proxy_validation(self):
        self.assertEqual(guard.validate_proxy('http://user:p%40ss@127.0.0.1:8080').port, 8080)
        for value in ['http://localhost:80', 'http://127.0.0.1', 'http://127.0.0.1:0', 'http://127.0.0.1:80/', 'http://127.0.0.1:80?', 'http://127.0.0.1:80#', 'http://[::1]:80', 'http://127.0.0.1:80\n', 'http://u%xx@127.0.0.1:80', 'socks5://127.0.0.1:80']:
            with self.subTest(value=value), self.assertRaises(ValueError):
                guard.validate_proxy(value)

    def test_roots_and_environment(self):
        for root in ['/', '/Users', os.path.expanduser('~'), '/Applications', '/System', '/Library', '/usr', '/opt', '/private', '/var', '/tmp', '/missing-not-real']:
            with self.subTest(root=root), self.assertRaises(ValueError):
                guard.make_profile(1, [root], [])
        with tempfile.TemporaryDirectory(dir=HERE) as root:
            root = os.path.realpath(root)
            for port in (0, 65536, True, '8080'):
                with self.assertRaises(ValueError):
                    guard.make_profile(port, [root], [])
            with mock.patch.dict(os.environ, {'no_proxy': '*', 'SSH_AUTH_SOCK': '/x', 'DYLD_INSERT_LIBRARIES': '/x', 'CUSTOM_PROXY': '/x', 'ZDOTDIR': '/x', 'npm_config_userconfig': '/x', 'BASH_FUNC_test%%': '() { :; }', 'UNRELATED_SECRET': 'not-forwarded'}):
                env = guard.proxy_env('http://127.0.0.1:1', root, root)
            for key in ('no_proxy', 'SSH_AUTH_SOCK', 'DYLD_INSERT_LIBRARIES', 'CUSTOM_PROXY', 'ZDOTDIR', 'npm_config_userconfig', 'BASH_FUNC_test%%', 'UNRELATED_SECRET'):
                self.assertNotIn(key, env)
            self.assertEqual(env['HOME'], root)
            self.assertEqual(env['HTTP_PROXY'], 'http://127.0.0.1:1')

    @unittest.skipUnless(sys.platform == 'darwin', 'requires macOS sandbox')
    def test_real_sandbox(self):
        with tempfile.TemporaryDirectory(dir=HERE) as base, socket.socket() as allowed, socket.socket() as blocked:
            base = os.path.realpath(base)
            root = os.path.join(base, 'root')
            os.mkdir(root)
            protected = os.path.join(root, 'protected')
            os.mkdir(protected)
            with open(os.path.join(protected, 'file'), 'w') as f:
                f.write('safe')
            for listener in (allowed, blocked):
                listener.bind(('127.0.0.1', 0))
                listener.listen(4)
            lib = ctypes.CDLL('/usr/lib/libSystem.B.dylib')
            bootstrap = ctypes.c_uint.in_dll(lib, 'bootstrap_port').value
            service = ctypes.c_uint()
            self.assertEqual(lib.bootstrap_look_up(bootstrap, b'com.apple.cfprefsd.daemon', ctypes.byref(service)), 0)
            port = allowed.getsockname()[1]
            other = blocked.getsockname()[1]
            unix_path = os.path.join(base, 'sock')
            with socket.socket(socket.AF_INET6, socket.SOCK_STREAM) as ipv6, socket.socket(socket.AF_INET6, socket.SOCK_STREAM) as other6, socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as unix_listener:
                ipv6.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
                ipv6.bind(('::1', port))
                ipv6.listen(1)
                other6.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
                other6.bind(('::1', other))
                other6.listen(1)
                unix_listener.bind(unix_path)
                unix_listener.listen(1)
                self._sandbox_checks(root, base, protected, port, other, unix_path)

    def _sandbox_checks(self, root, base, protected, port, other, unix_path):
            profile = guard.make_profile(port, [root], [protected])
            code = r'''
import ctypes,os,socket,sys,subprocess
lib=ctypes.CDLL('/usr/lib/libSystem.B.dylib')
bootstrap=ctypes.c_uint.in_dll(lib,'bootstrap_port').value
service=ctypes.c_uint()
assert lib.bootstrap_look_up(bootstrap,b'com.apple.cfprefsd.daemon',ctypes.byref(service)) != 0, 'Mach lookup allowed'
root,base,port,other,unix_path=sys.argv[1],sys.argv[2],int(sys.argv[3]),int(sys.argv[4]),sys.argv[5]
def must_fail(fn):
 try: fn()
 except OSError: return
 raise AssertionError('operation unexpectedly allowed')
def connect(family,typ,address):
 s=socket.socket(family,typ)
 try: s.connect(address)
 finally: s.close()
connect(socket.AF_INET,socket.SOCK_STREAM,('127.0.0.1',port))
must_fail(lambda:connect(socket.AF_INET,socket.SOCK_STREAM,('127.0.0.1',other)))
connect(socket.AF_INET6,socket.SOCK_STREAM,('::1',port)) # Both families are trusted relay endpoints.
must_fail(lambda:connect(socket.AF_INET6,socket.SOCK_STREAM,('::1',other)))
must_fail(lambda:connect(socket.AF_INET,socket.SOCK_DGRAM,('127.0.0.1',port)))
def bind():
 s=socket.socket()
 try: s.bind(('127.0.0.1',0))
 finally: s.close()
must_fail(bind)
must_fail(lambda:connect(socket.AF_INET,socket.SOCK_DGRAM,('127.0.0.1',53)))
must_fail(lambda:connect(socket.AF_UNIX,socket.SOCK_STREAM,unix_path))
with open(os.path.join(root,'ok'),'w') as f: f.write('ok')
must_fail(lambda:open(os.path.join(base,'outside'),'w'))
must_fail(lambda:os.rename(os.path.join(root,'protected'),os.path.join(root,'renamed')))
must_fail(lambda:open(os.path.join(root,'protected','file'),'w'))
code="import socket; s=socket.socket(); s.connect(('127.0.0.1',%d))" % other
p=subprocess.run([sys.executable,'-c',code],env={},stdout=subprocess.PIPE,stderr=subprocess.PIPE)
assert p.returncode != 0, 'grandchild escaped'
print('sandbox checks passed')
'''
            env = guard.proxy_env('http://127.0.0.1:{}'.format(port), root, root)
            result = subprocess.run(guard.sandbox_argv(profile, [PYTHON, '-c', code, root, base, str(port), str(other), unix_path]), env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('sandbox checks passed', result.stdout)
            bash = subprocess.run(guard.sandbox_argv(profile, ['/bin/bash', '-c', 'echo bash-ok']), env=env, text=True, capture_output=True)
            self.assertEqual(bash.returncode, 0, bash.stderr)
            self.assertIn('bash-ok', bash.stdout)
            with open(os.path.join(root, 'fd-test'), 'w') as fd:
                cli = subprocess.run([PYTHON, os.path.join(HERE, 'guard.py'), '--proxy', 'http://127.0.0.1:{}'.format(port), '--home', root, '--tmp', root, '--', PYTHON, '-c', 'import os,sys; fd=int(sys.argv[1]);\ntry: os.fstat(fd)\nexcept OSError: sys.exit(0)\nsys.exit(1)', str(fd.fileno())], pass_fds=(fd.fileno(),), env=env, text=True, capture_output=True)
            self.assertEqual(cli.returncode, 0, cli.stderr)
            node = shutil.which('node')
            if node:
                result = subprocess.run(guard.sandbox_argv(profile, [os.path.realpath(node), '-e', 'console.log("node-ok")']), env=env, text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('node-ok', result.stdout)


if __name__ == '__main__':
    unittest.main(verbosity=2)
