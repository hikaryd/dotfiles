import fcntl
import json
import os
from pathlib import Path
import re
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import time

from guard import close_extra_fds, make_profile, proxy_env, sandbox_argv, validate_proxy, validate_stdio
from relay import fingerprint

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
    result = json.loads(CONFIG.read_text())
    if not isinstance(result, dict):
        raise ValueError("settings.json must contain an object")
    return result


def control(command="status"):
    with socket.socket(socket.AF_UNIX) as connection:
        connection.settimeout(2)
        connection.connect(str(STATE / "control.sock"))
        connection.sendall(command.encode() + b"\n")
        if command == "status":
            return json.loads(connection.recv(4096))


def relay(url):
    validate_proxy(url)
    with open(STATE / "launch.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        try:
            current = control()
        except (OSError, ValueError):
            code = "import runpy,sys;sys.path.insert(0,sys.argv.pop(1));runpy.run_module('relay',run_name='__main__')"
            with open(STATE / "relay.log", "ab") as log:
                process = subprocess.Popen([sys.executable, "-I", "-c", code, str(ROOT), str(STATE)],
                                           stdin=subprocess.PIPE, stdout=log, stderr=log,
                                           start_new_session=True, close_fds=True,
                                           env={"HOME": str(REAL_HOME), "PATH": "/usr/bin:/bin"})
                process.stdin.write(json.dumps({"proxy_url": url}).encode() + b"\n")
                process.stdin.close()
            for _ in range(50):
                try:
                    current = control()
                    break
                except (OSError, ValueError):
                    if process.poll() is not None:
                        raise RuntimeError("Proxy relay failed closed, see state/relay.log")
                    time.sleep(0.1)
            else:
                process.terminate()
                process.wait(timeout=5)
                raise RuntimeError("Proxy relay did not become ready")
        if current.get("fingerprint") != fingerprint(url):
            raise RuntimeError("Proxy settings changed. Close Claude sessions, run claude-setup relay-stop, then restart")
        port = current.get("port")
        if type(port) is not int or not 1 <= port <= 65535:
            raise RuntimeError("Invalid relay response")
        return port


def local_proxy(url, port):
    proxy = validate_proxy(url)
    auth = proxy.netloc.rsplit("@", 1)[0] + "@" if "@" in proxy.netloc else ""
    return "http://{}127.0.0.1:{}".format(auth, port)


def workspace_roots(config):
    items = config.get("workspaces", [])
    if not isinstance(items, list) or not all(isinstance(item, str) for item in items):
        raise ValueError("workspaces must be an array of directory paths")
    return [Path(item).expanduser().resolve(strict=True) for item in items]


def environment(url, port):
    env = proxy_env(local_proxy(url, port), HOME, TMP)
    env.update({"DISABLE_AUTOUPDATER": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
                "CLAUDE_CODE_CERT_STORE": "bundled", "CLAUDE_CONFIG_DIR": str(HOME / ".claude"),
                "NODE_USE_ENV_PROXY": "1", "NPM_CONFIG_USERCONFIG": "/dev/null",
                "NPM_CONFIG_GLOBALCONFIG": str(ROOT / "npm-globalrc"), "NPM_CONFIG_REGISTRY": "https://registry.npmjs.org",
                "NPM_CONFIG_CACHE": str(HOME / ".npm")})
    return env


def profile(port, workspaces=(), installing=None):
    writes = [HOME, TMP] + list(workspaces)
    protected = [ROOT, CONFIG.parent, STATE, VERSIONS]
    if installing:
        writes.append(installing)
    return make_profile(port, writes, protected, validate_stdio())


def run_claude(arguments, config):
    url = config.get("proxy_url", "")
    validate_proxy(url)
    roots = workspace_roots(config)
    cwd = Path.cwd().resolve()
    if not any(cwd == root or root in cwd.parents for root in roots):
        raise ValueError("Current directory is not in settings.json workspaces. Add the project explicitly")
    current = BASE / "current"
    binary = (current / "node_modules/.bin/claude").resolve(strict=True)
    if VERSIONS.resolve() not in binary.parents:
        raise RuntimeError("Refusing Claude executable outside installed versions")
    if arguments and arguments[0] in ("install", "update"):
        raise ValueError("Use claude-setup install for guarded updates")
    port = relay(url)
    command = sandbox_argv(profile(port, roots), [str(binary)] + arguments)
    env = environment(url, port)
    close_extra_fds()
    os.execve(command[0], command, env)


def install_claude(config):
    url = config.get("proxy_url", "")
    validate_proxy(url)
    version = config.get("claude_version", "")
    if not isinstance(version, str) or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("claude_version must be an explicit x.y.z version")
    node, npm = shutil.which("node"), shutil.which("npm")
    if not node or not npm:
        raise RuntimeError("Install Node.js and npm before installing Claude")
    destination = VERSIONS / version
    with open(STATE / "install.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if not destination.exists():
            port = relay(url)
            stage = Path(tempfile.mkdtemp(prefix="claude-stage-", dir=str(TMP)))
            try:
                argv = [node, str(Path(npm).resolve()), "install", "--prefix", str(stage),
                        "--no-audit", "--no-fund", "--include=optional",
                        "@anthropic-ai/claude-code@" + version]
                subprocess.run(sandbox_argv(profile(port, installing=stage), argv),
                               env=environment(url, port), cwd=stage, check=True, close_fds=True)
                binary = (stage / "node_modules/.bin/claude").resolve(strict=True)
                if stage not in binary.parents or not binary.is_file():
                    raise RuntimeError("Installed Claude binary is outside the staging directory")
                subprocess.run(sandbox_argv(profile(port, installing=stage), [str(binary), "--version"]),
                               env=environment(url, port), cwd=stage, check=True, close_fds=True)
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
    print("Claude installed under guard. Run claude from an allowed project directory.")


def main():
    os.umask(0o077)
    try:
        close_extra_fds()
        mode, *arguments = sys.argv[1:]
        if mode == "claude-setup" and arguments == ["settings"]:
            print(CONFIG)
            return
        if mode == "claude-setup" and arguments == ["relay-stop"]:
            control("stop")
            print("Relay stopped. Restart existing Claude sessions before using another proxy.")
            return
        config = settings()
        if mode == "claude":
            run_claude(arguments, config)
        elif mode == "claude-setup" and arguments == ["install"]:
            install_claude(config)
        elif mode == "claude-setup" and arguments == ["check"]:
            validate_proxy(config.get("proxy_url", ""))
            workspace_roots(config)
            print("Configuration valid. This does not certify live Claude/browser traffic.")
        elif mode == "claude-browser":
            from browser import main as browser_main
            browser_main(config, ROOT, STATE, arguments)
        else:
            print("Usage: claude [args] | claude-setup settings|check|install|relay-stop | claude-browser prepare|start|stop|status", file=sys.stderr)
            sys.exit(2)
    except (ValueError, OSError, RuntimeError, subprocess.CalledProcessError) as error:
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
