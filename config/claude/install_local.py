import json
import os
from pathlib import Path
import shlex
import sys

SOURCE = Path(__file__).resolve().parent
FILES = ("guard.py", "launcher.py", "relay.py", "browser.py", "firewall.sh", "browser-start.sh",
         "session-hook.py", "session-context.txt", "test_guard.py", "test_setup.py", "test_browser.py", "test_browser_network.py",
         "install_local.py", "settings.example.json")
EXCLUDED_SKILLS = {"chief-of-staff", "claude-handoff", "peekaboo"}
MARKER = "dots-claude-guard-v1\n"


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
    temp = path.with_name(path.name + ".new")
    with open(temp, "x") as stream:
        stream.write(contents)
    temp.chmod(mode)
    temp.replace(path)


def install(home):
    home = home.resolve(strict=True)
    base = home / ".local/share/claude-guard"
    marker = base / ".owner"
    if base.exists() and (not marker.is_file() or marker.read_text() != MARKER):
        raise RuntimeError("Refusing to adopt existing unowned claude-guard directory")
    for name in FILES:
        if not (SOURCE / name).is_file():
            raise RuntimeError("Missing guard source: " + name)
    for directory in (base, base / "runtime", base / "state", base / "home", base / "tmp", base / "versions",
                      home / ".config/claude-guard", base / "home/.claude", base / "home/.claude/skills"):
        private_directory(directory)
    if not marker.exists():
        write_owned(marker, MARKER)
    runtime = base / "runtime"
    for name in FILES:
        write_owned(runtime / name, (SOURCE / name).read_text(), 0o644 if name.endswith(".sh") else 0o600)
    config = home / ".config/claude-guard/settings.json"
    if not config.exists():
        defaults = json.loads((SOURCE / "settings.example.json").read_text())
        projects = home / "Documents/dev"
        if projects.is_dir():
            defaults["workspaces"] = [str(projects.resolve())]
        write_owned(config, json.dumps(defaults, indent=2) + "\n")
    elif config.is_symlink() or config.stat().st_uid != os.getuid() or config.stat().st_mode & 0o077:
        raise RuntimeError("Existing settings.json must be an owned regular mode-600 file")
    destinations = base / "home/.claude/skills"
    linked = []
    for source in (home / ".agents/skills", home / ".jcode/skills", home / ".claude/skills"):
        if not source.is_dir():
            continue
        for skill in sorted(source.iterdir()):
            if skill.name in EXCLUDED_SKILLS or not (skill / "SKILL.md").is_file():
                continue
            target = destinations / skill.name
            if target.exists() or target.is_symlink():
                if target.is_symlink() and target.resolve() == skill.resolve():
                    continue
                raise RuntimeError("Refusing to replace existing skill: " + skill.name)
            target.symlink_to(skill.resolve(), target_is_directory=True)
            linked.append(skill.name)
    claude_settings = base / "home/.claude/settings.json"
    if not claude_settings.exists():
        hook = shlex.join([sys.executable, "-I", str(runtime / "session-hook.py")])
        settings = {"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": hook}]}]}}
        write_owned(claude_settings, json.dumps(settings, indent=2) + "\n")
    bindir = home / ".local/bin"
    bindir.mkdir(parents=True, exist_ok=True)
    for command in ("claude", "claude-setup", "claude-browser"):
        target = bindir / command
        if target.exists() or target.is_symlink():
            if target.is_symlink() or "# dots-claude-guard-v1" not in target.read_text():
                raise RuntimeError("Refusing to overwrite existing command: " + command)
        code = "import sys;sys.path.insert(0,{});from launcher import main;main()".format(repr(str(runtime)))
        script = "#!/bin/sh\n# dots-claude-guard-v1\nexec /usr/bin/env -i HOME={} PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin TERM=\"${{TERM:-xterm-256color}}\" LANG=\"${{LANG:-en_US.UTF-8}}\" {} -I -c {} {} \"$@\"\n".format(
            shlex.quote(str(home)), shlex.quote(sys.executable), shlex.quote(code), shlex.quote(command))
        write_owned(target, script, 0o700)
    print("Installed local guard and {} new skill links. No packages downloaded.".format(len(linked)))
    print("Set proxy_url in " + str(config))
    print("proxy_url: http(s)://[user:password@]NUMERIC_IPV4:PORT, no path or bypass list.")
    print("Then: claude-setup install; claude-browser prepare; claude-browser start")
    print("Not yet validated with actual Claude login or browser UI. Raw binaries/host apps are outside this boundary.")


if __name__ == "__main__":
    os.umask(0o077)
    try:
        if len(sys.argv) != 1:
            raise ValueError("No arguments expected")
        install(Path.home())
    except (OSError, RuntimeError, ValueError) as error:
        print("claude setup: " + str(error), file=sys.stderr)
        sys.exit(1)
