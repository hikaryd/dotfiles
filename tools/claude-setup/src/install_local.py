import json
import fcntl
import os
from pathlib import Path
import re
import shlex
import stat
import subprocess
import sys
import tempfile

SOURCE = Path(__file__).resolve().parent
FILES = ("guard.py", "launcher.py", "donut.py", "relay.py",
         "install_local.py", "settings.example.json",
         "npm-globalrc", "webrtc-manifest.json", "webrtc-policy.js", "bash-env.sh")
RETIRED = ("browser.py", "firewall.sh", "browser-start.sh", "session-hook.py", "ssh_proxy.py",
           "session-context.txt", "test_guard.py", "test_setup.py", "test_browser.py",
           "test_browser_network.py", "firefox-prefs.js", "firefox.cfg",
           "test_firefox_auth.js", "test_firefox_live.py", "test_vnc_startup.py")
RETIRED_CONFIG = ("browser_base_image", "browser_cpus", "browser_memory_mb",
                  "browser_port", "browser_proxy_url")
MARKER = "dots-claude-guard-v1\n"


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("Duplicate JSON key")
        result[key] = value
    return result


def reject_constant(value):
    raise ValueError("Nonstandard JSON constant: " + value)


def open_private(path, append=False):
    flags = os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK
    if append:
        flags |= os.O_APPEND
    descriptor = os.open(path, flags, 0o600)
    try:
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077 or info.st_nlink != 1:
            raise RuntimeError("Unsafe private file: " + str(path))
        return os.fdopen(descriptor, "a+b" if append else "r+b")
    except BaseException:
        os.close(descriptor)
        raise


def private_directory(path):
    if path.is_symlink():
        raise RuntimeError("Refusing symlink directory: " + str(path))
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    if path.stat().st_uid != os.getuid():
        raise RuntimeError("Refusing unowned directory: " + str(path))
    path.chmod(0o700)


def write_owned(path, contents, mode=0o600):
    if path.is_symlink():
        raise RuntimeError("Refusing symlink file: " + str(path))
    descriptor, name = tempfile.mkstemp(prefix=".claude-guard-", dir=path.parent)
    temp = Path(name)
    try:
        with os.fdopen(descriptor, "w") as stream:
            stream.write(contents)
        temp.chmod(mode)
        temp.replace(path)
    finally:
        temp.unlink(missing_ok=True)


def check_path(home, path, directory=False, private=False):
    current = home
    for part in path.relative_to(home).parts:
        current = current / part
        try:
            info = current.lstat()
        except FileNotFoundError:
            return
        is_directory = current != path or directory
        if (info.st_uid != os.getuid() or
                not (stat.S_ISDIR(info.st_mode) if is_directory else stat.S_ISREG(info.st_mode))):
            raise RuntimeError("Unsafe path: " + str(current))
        if is_directory and info.st_mode & 0o022:
            raise RuntimeError("Каталог доступен для записи другим пользователям: " + str(current))
        if current == path and private and info.st_mode & 0o077:
            raise RuntimeError("Файл должен быть доступен только владельцу: " + str(current))


def preflight(home, base):
    info = home.stat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise RuntimeError("Unsafe home directory")
    directories = [base, base / "runtime", base / "runtime/host-bin", base / "state",
                   base / "home", base / "tmp", base / "versions", base / "home/.claude",
                   home / ".config/claude-guard", home / ".local/bin"]
    for directory in directories:
        check_path(home, directory, directory=True)
    files = [base / ".owner", base / "runtime/host-bin/ps", base / "runtime/host-bin/ssh",
             base / "home/.claude/settings.json"] + [base / "runtime" / name for name in FILES + RETIRED]
    for path in files:
        check_path(home, path)
    config = home / ".config/claude-guard/settings.json"
    check_path(home, config, private=True)
    for path in [config, base / "home/.claude/settings.json"] + [base / "runtime" / name for name in RETIRED]:
        if path.exists() and path.stat().st_nlink != 1:
            raise RuntimeError("Unsafe linked file: " + str(path))
    if config.exists():
        try:
            if not isinstance(json.loads(config.read_text(), object_pairs_hook=unique_object, parse_constant=reject_constant), dict):
                raise ValueError()
        except (ValueError, UnicodeError):
            raise RuntimeError("Некорректный settings.json") from None
    marker = base / ".owner"
    if base.exists() and (not marker.exists() or marker.read_text() != MARKER):
        raise RuntimeError("Refusing to adopt existing unowned claude-guard directory")
    for command in ("claude", "claude-setup", "claude-browser"):
        target = home / ".local/bin" / command
        check_path(home, target)
        if target.exists() and "# dots-claude-guard-v1" not in target.read_text():
            raise RuntimeError("Refusing to overwrite existing command: " + command)
    for name in FILES:
        if not (SOURCE / name).is_file():
            raise RuntimeError("Missing guard source: " + name)
    return migration(home, base)


def migration(home, base):
    config = home / ".config/claude-guard/settings.json"
    settings_path = base / "home/.claude/settings.json"
    config_data = json.loads(config.read_text(), object_pairs_hook=unique_object, parse_constant=reject_constant) if config.exists() else None
    if config_data is not None:
        config_data = {key: value for key, value in config_data.items() if key not in RETIRED_CONFIG}
    if not settings_path.exists():
        return config_data, None
    try:
        settings = json.loads(settings_path.read_text(), object_pairs_hook=unique_object, parse_constant=reject_constant)
        if not isinstance(settings, dict):
            raise ValueError()
    except (ValueError, UnicodeError):
        raise RuntimeError("Некорректный Claude settings.json") from None

    target = str(base / "runtime/session-hook.py")
    changed = False
    hooks = settings.get("hooks", {})
    if isinstance(hooks, dict) and isinstance(hooks.get("SessionStart"), list):
        groups = []
        for group in hooks["SessionStart"]:
            if not isinstance(group, dict) or not isinstance(group.get("hooks"), list):
                groups.append(group)
                continue
            kept = []
            for hook in group["hooks"]:
                command = hook.get("command") if isinstance(hook, dict) else None
                try:
                    parts = shlex.split(command) if isinstance(command, str) else []
                except ValueError:
                    parts = []
                owned = (isinstance(hook, dict) and set(hook) == {"type", "command"} and hook.get("type") == "command" and
                         len(parts) == 3 and Path(parts[0]).is_absolute() and
                         re.fullmatch(r"python(?:\d+(?:\.\d+)*)?", Path(parts[0]).name) and
                         parts[1:] == ["-I", target] and
                         command == shlex.join(parts))
                if owned:
                    changed = True
                else:
                    kept.append(hook)
            if kept or not group["hooks"] or set(group) != {"hooks"}:
                groups.append({**group, "hooks": kept})
        hooks["SessionStart"] = groups

    def has_reference(value):
        if isinstance(value, str):
            if target in value or shlex.quote(target) in value:
                return True
            try:
                return any(target in token for token in shlex.split(value))
            except ValueError:
                return False
        if isinstance(value, list):
            return any(has_reference(item) for item in value)
        if isinstance(value, dict):
            return any(has_reference(key) or has_reference(item) for key, item in value.items())
        return False

    if has_reference(settings):
        raise RuntimeError("Ambiguous retired session hook reference in Claude settings")
    return config_data, settings if changed else None


def install_host_ps(runtime):
    directory = runtime / "host-bin"
    private_directory(directory)
    target = directory / "ps"
    if target.is_symlink():
        raise RuntimeError("Refusing symlink file: " + str(target))
    descriptor, name = tempfile.mkstemp(prefix=".ps-", dir=directory)
    temporary = Path(name)
    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(Path("/bin/ps").read_bytes())
        temporary.chmod(0o700)
        # Seatbelt rejects setuid ps; a relocated system binary also needs a new signature.
        for arguments in (("--force", "--sign", "-", "--timestamp=none"), ("--verify", "--strict")):
            subprocess.run(["/usr/bin/codesign", *arguments, str(temporary)],
                           env={"PATH": "/usr/bin:/bin"}, stdin=subprocess.DEVNULL,
                           capture_output=True, check=True, timeout=15)
        temporary.replace(target)
    finally:
        temporary.unlink(missing_ok=True)


def install(home):
    home = home.resolve(strict=True)
    preflight(home, home / ".local/share/claude-guard")
    check_path(home, home / ".local", directory=True)
    (home / ".local").mkdir(exist_ok=True, mode=0o700)
    lock_path = home / ".local/claude-guard-install.lock"
    check_path(home, lock_path, private=True)
    with open_private(lock_path) as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        _install(home)


def _install(home):
    base = home / ".local/share/claude-guard"
    marker = base / ".owner"
    config_data, claude_settings = preflight(home, base)
    for directory in (base, base / "runtime", base / "state", base / "home", base / "tmp", base / "versions",
                      home / ".config/claude-guard", base / "home/.claude"):
        private_directory(directory)
    if not marker.exists():
        write_owned(marker, MARKER)
    runtime = base / "runtime"
    if claude_settings is not None:
        write_owned(base / "home/.claude/settings.json", json.dumps(claude_settings, indent=2) + "\n")
    for name in RETIRED:
        (runtime / name).unlink(missing_ok=True)
    install_host_ps(runtime)
    for name in FILES:
        write_owned(runtime / name, (SOURCE / name).read_text())
    write_owned(runtime / "host-bin/ssh", '#!/bin/sh\nexec /usr/bin/ssh "$@"\n', 0o700)
    config = home / ".config/claude-guard/settings.json"
    if not config.exists():
        defaults = json.loads((SOURCE / "settings.example.json").read_text())
        write_owned(config, json.dumps(defaults, indent=2) + "\n")
    elif any(key in json.loads(config.read_text()) for key in RETIRED_CONFIG):
        write_owned(config, json.dumps(config_data, indent=2) + "\n")
    elif config.is_symlink() or config.stat().st_uid != os.getuid() or config.stat().st_mode & 0o077:
        raise RuntimeError("Existing settings.json must be an owned regular mode-600 file")
    bindir = home / ".local/bin"
    bindir.mkdir(parents=True, exist_ok=True)
    for command in ("claude", "claude-setup", "claude-browser"):
        target = bindir / command
        if target.exists() or target.is_symlink():
            if target.is_symlink() or "# dots-claude-guard-v1" not in target.read_text():
                raise RuntimeError("Refusing to overwrite existing command: " + command)
        code = "import sys;sys.path.insert(0,{});from launcher import main;main()".format(repr(str(runtime)))
        script = "#!/bin/sh\n# dots-claude-guard-v1\nexec /usr/bin/env -i HOME={} PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin TERM=\"${{TERM:-xterm-256color}}\" LANG=\"${{LANG:-en_US.UTF-8}}\" SSH_AUTH_SOCK=\"${{SSH_AUTH_SOCK:-}}\" {} -I -c {} {} \"$@\"\n".format(
            shlex.quote(str(home)), shlex.quote(sys.executable), shlex.quote(code), shlex.quote(command))
        write_owned(target, script, 0o700)
    print("Локальная среда установлена. Пакеты не скачивались.")
    print('Добавьте команды в PATH для текущей оболочки: export PATH="$HOME/.local/bin:$PATH"')
    print("Далее: claude-setup configure; claude-setup check; claude-setup install")
    print("Node.js 18+ и npm должны быть в /opt/homebrew/bin, /usr/local/bin или системном PATH; NVM не поддерживается.")
    print("Для Donut включите Local API, затем выполните claude-browser configure и claude-browser prepare.")


if __name__ == "__main__":
    os.umask(0o077)
    try:
        if len(sys.argv) != 1:
            raise ValueError("No arguments expected")
        install(Path.home())
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print("claude setup: " + str(error), file=sys.stderr)
        sys.exit(1)
