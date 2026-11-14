import fcntl
import getpass
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import time
import warnings

import donut
from guard import close_extra_fds, make_host_profile, make_profile, proxy_env, sandbox_argv, validate_proxy, validate_stdio
from relay import fingerprint
from install_local import open_private, write_owned

ROOT = Path(__file__).resolve().parent
BASE = ROOT.parent
REAL_HOME = Path.home()
CONFIG = REAL_HOME / ".config/claude-guard/settings.json"
STATE = BASE / "state"
HOME = BASE / "home"
TMP = BASE / "tmp"
VERSIONS = BASE / "versions"


def settings():
    info = CONFIG.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise ValueError("settings.json must be an owned regular file with mode 600")
    try:
        result = json.loads(CONFIG.read_text())
    except (ValueError, UnicodeError):
        raise ValueError("Некорректный JSON в settings.json") from None
    if not isinstance(result, dict):
        raise ValueError("settings.json must contain an object")
    return result


def validate_version(version):
    if not isinstance(version, str) or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("claude_version: укажите точную версию x.y.z")
    return version


def configure():
    if not sys.stdin.isatty() or not sys.stdout.isatty():
        raise ValueError("configure требует интерактивный терминал")
    config = settings()
    try:
        with warnings.catch_warnings():
            warnings.simplefilter("error", getpass.GetPassWarning)
            url = getpass.getpass("Proxy URL (пусто: сохранить): ") or config.get("proxy_url", "")
    except getpass.GetPassWarning:
        raise ValueError("Не удалось отключить отображение ввода, настройки не изменены") from None
    version = input("Версия Claude x.y.z (пусто: сохранить): ") or config.get("claude_version", "")
    try:
        validate_proxy(url)
    except (ValueError, TypeError):
        raise ValueError("Некорректный proxy URL") from None
    validate_version(version)
    config.update(proxy_url=url, claude_version=version)
    write_owned(CONFIG, json.dumps(config, indent=2) + "\n")
    print("Настройки сохранены.")


def node_tools():
    node, npm = shutil.which("node"), shutil.which("npm")
    if not node or not npm:
        raise RuntimeError("Установите Node.js 18+ и npm")
    result = subprocess.run([node, "--version"], capture_output=True, text=True, check=True, timeout=10)
    match = re.fullmatch(r"v([0-9]+)\.[0-9]+\.[0-9]+\s*", result.stdout)
    if not match or int(match.group(1)) < 18:
        raise RuntimeError("Требуется Node.js 18 или новее")
    return node, npm


def check(config):
    try:
        validate_proxy(config.get("proxy_url", ""))
    except (ValueError, TypeError):
        raise ValueError("Некорректный proxy URL. Выполните claude-setup configure") from None
    validate_version(config.get("claude_version", ""))
    if len(os.fsencode(STATE / ("relay-" + "0" * 24) / "control.sock")) >= 104:
        raise RuntimeError("Путь домашнего каталога слишком длинный для Unix-сокета relay. Используйте более короткий путь к домашнему каталогу.")
    for path in (ROOT / "guard.py", ROOT / "relay.py", ROOT / "ssh_proxy.py", ROOT / "host-bin/ps", ROOT / "host-bin/ssh", Path("/usr/bin/sandbox-exec")):
        if not path.is_file():
            raise RuntimeError("Не найден необходимый компонент: " + str(path))
    node_tools()
    try:
        binary = (BASE / "current/node_modules/.bin/claude").resolve(strict=True)
        installed = VERSIONS.resolve() in binary.parents and binary.is_file()
    except OSError:
        installed = False
    print("Проверка без сети пройдена. " + ("Claude установлен." if installed else "Среда готова, Claude не установлен. Выполните claude-setup install."))


def private_relay_state(state):
    info = state.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise RuntimeError("Relay state must be an owned private directory")


def control(command="status", state=None):
    state = STATE if state is None else state
    private_relay_state(state)
    path = state / "control.sock"
    info = path.lstat()
    if not stat.S_ISSOCK(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise RuntimeError("Refusing an unsafe relay control socket")
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(2)
        connection.connect(str(path))
        connection.sendall(command.encode() + b"\n")
        if command == "status":
            with connection.makefile("rb") as stream:
                response = stream.readline(4097)
            if len(response) > 4096 or not response.endswith(b"\n"):
                raise RuntimeError("Invalid relay response")
            result = json.loads(response)
            if (not isinstance(result, dict) or type(result.get("port")) is not int
                    or not 1 <= result["port"] <= 65535
                    or not isinstance(result.get("fingerprint"), str)
                    or not re.fullmatch(r"[0-9a-f]{64}", result["fingerprint"])):
                raise RuntimeError("Invalid relay response")
            return result


def relay(url):
    validate_proxy(url)
    digest = fingerprint(url)
    private_relay_state(STATE)
    with open_private(STATE / "launch.lock") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            current = control()
        except (FileNotFoundError, ConnectionRefusedError):
            current = None
        if current and current["fingerprint"] == digest:
            return current["port"]
        # Keep existing sessions on their original endpoint and credentials.
        # Keep socket names short; verify the full digest before reuse.
        state = STATE / ("relay-" + digest[:24])
        if len(os.fsencode(state / "control.sock")) >= 104:
            raise RuntimeError("Relay state path is too long for a macOS local socket")
        state.mkdir(mode=0o700, exist_ok=True)
        private_relay_state(state)
        try:
            current = control(state=state)
        except (FileNotFoundError, ConnectionRefusedError):
            code = "import runpy,sys;sys.path.insert(0,sys.argv.pop(1));runpy.run_module('relay',run_name='__main__')"
            with open_private(STATE / "relay.log", append=True) as log:
                process = subprocess.Popen([sys.executable, "-I", "-c", code, str(ROOT), str(state)],
                                           stdin=subprocess.PIPE, stdout=log, stderr=log,
                                           start_new_session=True, close_fds=True,
                                           env={"HOME": str(REAL_HOME), "PATH": "/usr/bin:/bin"})
                process.stdin.write(json.dumps({"proxy_url": url}).encode() + b"\n")
                process.stdin.close()
            for _ in range(50):
                try:
                    current = control(state=state)
                    break
                except (OSError, ValueError):
                    if process.poll() is not None:
                        raise RuntimeError("Proxy relay failed closed, see state/relay.log")
                    time.sleep(0.1)
            else:
                process.terminate()
                process.wait(timeout=5)
                raise RuntimeError("Proxy relay did not become ready")
        if current["fingerprint"] != digest:
            raise RuntimeError("Relay configuration mismatch. No unguarded retry")
        return current["port"]


def stop_relays():
    private_relay_state(STATE)
    with open_private(STATE / "launch.lock") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        states = [STATE] + sorted(path for path in STATE.iterdir()
                                  if re.fullmatch(r"relay-[0-9a-f]{24}", path.name))
        for state in states:
            private_relay_state(state)
        for state in states:
            try:
                control("stop", state)
            except (FileNotFoundError, ConnectionRefusedError):
                pass
            try:
                descriptor = os.open(state / "service.lock", os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK)
            except FileNotFoundError:
                continue
            with os.fdopen(descriptor, "r+") as service:
                info = os.fstat(service.fileno())
                if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
                    raise RuntimeError("Refusing an unsafe relay service lock")
                # The service releases this lock only after closing its listeners.
                for _ in range(50):
                    try:
                        fcntl.flock(service, fcntl.LOCK_EX | fcntl.LOCK_NB)
                        break
                    except BlockingIOError:
                        time.sleep(0.1)
                else:
                    raise RuntimeError("Relay did not stop in time. No process was force-killed")


def local_proxy(url, port):
    proxy = validate_proxy(url)
    auth = proxy.netloc.rsplit("@", 1)[0] + "@" if "@" in proxy.netloc else ""
    return "http://{}127.0.0.1:{}".format(auth, port)


def environment(url, port, *, host_access=False):
    env = proxy_env(local_proxy(url, port), HOME, TMP)
    env.update({"DISABLE_AUTOUPDATER": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
                "TZ": "Europe/London", "LANG": "en_GB.UTF-8", "LC_ALL": "en_GB.UTF-8",
                "CLAUDE_CODE_CERT_STORE": "bundled", "CLAUDE_CONFIG_DIR": str(HOME / ".claude"),
                "SSL_CERT_FILE": "/etc/ssl/cert.pem", "GODEBUG": "x509sslcertoverrideplatform=1",
                "CLAUDE_CODE_TMPDIR": str(TMP),
                "NODE_USE_ENV_PROXY": "1", "NPM_CONFIG_USERCONFIG": "/dev/null",
                "NPM_CONFIG_GLOBALCONFIG": str(ROOT / "npm-globalrc"), "NPM_CONFIG_REGISTRY": "https://registry.npmjs.org",
                "NPM_CONFIG_CACHE": str(HOME / ".npm"),
                "DO_NOT_TRACK": "1"})
    if host_access:
        # Preserve existing Claude state while exposing the real home to its tools.
        env["HOME"] = str(REAL_HOME)
        env["PATH"] = os.pathsep.join([str(ROOT / "host-bin"), str(REAL_HOME / ".local/bin"), env["PATH"]])
        env["GIT_SSH_COMMAND"] = shlex.quote(str(ROOT / "host-bin/ssh"))
        if os.environ.get("SSH_AUTH_SOCK"):
            env["SSH_AUTH_SOCK"] = os.environ["SSH_AUTH_SOCK"]
    return env


def profile(port, working_directory=None, installing=None):
    if working_directory is not None and not installing:
        return make_host_profile()
    writes = [HOME, TMP]
    protected = [ROOT, CONFIG.parent, STATE, VERSIONS]
    protected.extend(REAL_HOME / ".local/bin" / name for name in ("claude", "claude-setup", "claude-browser"))
    if installing:
        writes.append(installing)
    return make_profile(port, writes, protected, validate_stdio(), working_directory=working_directory)


def run_claude(arguments, config):
    url = config.get("proxy_url", "")
    validate_proxy(url)
    cwd = Path.cwd().resolve(strict=True)
    current = BASE / "current"
    binary = (current / "node_modules/.bin/claude").resolve(strict=True)
    if VERSIONS.resolve() not in binary.parents:
        raise RuntimeError("Refusing Claude executable outside installed versions")
    if arguments and arguments[0] in ("install", "update"):
        raise ValueError("Use claude-setup install for guarded updates")
    port = relay(url)
    command = sandbox_argv(profile(port, working_directory=cwd), [str(binary)] + arguments)
    env = environment(url, port, host_access=True)
    close_extra_fds()
    os.execve(command[0], command, env)


def install_claude(config):
    url = config.get("proxy_url", "")
    validate_proxy(url)
    version = validate_version(config.get("claude_version", ""))
    node, npm = node_tools()
    destination = VERSIONS / version
    with open_private(STATE / "install.lock") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if not destination.exists():
            port = relay(url)
            stage = Path(tempfile.mkdtemp(prefix="claude-stage-", dir=str(TMP)))
            try:
                argv = [node, str(Path(npm).resolve()), "install", "--prefix", str(stage),
                        "--no-audit", "--no-fund", "--include=optional",
                        "@anthropic-ai/claude-code@" + version]
                env = environment(url, port)
                env["PATH"] = str(Path(node).parent) + os.pathsep + env["PATH"]
                subprocess.run(sandbox_argv(profile(port, installing=stage), argv),
                               env=env, cwd=stage, check=True, close_fds=True)
                binary = (stage / "node_modules/.bin/claude").resolve(strict=True)
                if stage not in binary.parents or not binary.is_file():
                    raise RuntimeError("Installed Claude binary is outside the staging directory")
                subprocess.run(sandbox_argv(profile(port, installing=stage), [str(binary), "--version"]),
                               env=env, cwd=stage, check=True, close_fds=True)
                stage.rename(destination)
            finally:
                if stage.exists():
                    shutil.rmtree(stage)
        binary = (destination / "node_modules/.bin/claude").resolve(strict=True)
        if destination not in binary.parents:
            raise RuntimeError("Invalid installed binary")
        link = BASE / "current.new"
        if link.exists() or link.is_symlink():
            link.unlink()
        link.symlink_to(destination)
        link.replace(BASE / "current")
    print("Claude installed under guard. Run claude from the project directory you want to work in.")


def run_browser(arguments):
    if arguments in (["--help"], ["-h"]):
        print("Usage: claude-browser [configure|start|prepare|status]")
        return
    if arguments == ["configure"]:
        donut.configure()
        return
    if arguments == ["status"]:
        donut.status()
        return
    if arguments == ["stop"]:
        raise ValueError("Остановите нужный профиль в Donut. Остальные профили не будут закрыты")
    if arguments not in ([], ["start"], ["prepare"]):
        raise ValueError("Usage: claude-browser [configure|start|prepare|status]")
    application = Path("/Applications/Donut.app")
    if not application.is_dir():
        application = REAL_HOME / "Applications/Donut.app"
    if not application.is_dir():
        raise RuntimeError("Install the native Donut application first")
    try:
        subprocess.run(["/usr/bin/open", "-a", str(application)], check=True)
    except subprocess.CalledProcessError as error:
        raise RuntimeError("Native Donut failed to open (exit {})".format(error.returncode)) from None
    donut.prepare(settings().get("proxy_url", ""))


def main():
    os.umask(0o077)
    try:
        close_extra_fds()
        mode, *arguments = sys.argv[1:]
        if mode == "claude-setup" and arguments in ([], ["--help"], ["-h"]):
            print("Использование: claude-setup configure|settings|check|install|relay-stop")
            return
        if mode == "claude-setup" and arguments == ["configure"]:
            configure()
            return
        if mode == "claude-setup" and arguments == ["settings"]:
            print(CONFIG)
            return
        if mode == "claude-setup" and arguments == ["relay-stop"]:
            stop_relays()
            print("Relays stopped. Existing Claude sessions must be restarted to reconnect.")
            return
        if mode == "claude-browser":
            run_browser(arguments)
            return
        if mode == "claude-setup" and arguments not in (["install"], ["check"]):
            print("Использование: claude-setup configure|settings|check|install|relay-stop", file=sys.stderr)
            sys.exit(2)
        config = settings()
        if mode == "claude":
            run_claude(arguments, config)
        elif mode == "claude-setup" and arguments == ["install"]:
            install_claude(config)
        elif mode == "claude-setup" and arguments == ["check"]:
            check(config)
        else:
            print("Usage: claude [args] | claude-setup settings|check|install|relay-stop | claude-browser start|status", file=sys.stderr)
            sys.exit(2)
    except (ValueError, OSError, RuntimeError, subprocess.SubprocessError, EOFError, KeyboardInterrupt) as error:
        # Validation errors never echo proxy credentials or subprocess arguments.
        if isinstance(error, subprocess.CalledProcessError):
            message = "Guarded command failed (exit {}). No unguarded retry.".format(error.returncode)
        elif isinstance(error, OSError):
            message = "Required local file, service or runtime is unavailable. Run claude-setup settings."
        else:
            message = str(error)
        print("claude-guard: " + message, file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
