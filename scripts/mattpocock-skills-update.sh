#!/usr/bin/env bash
set -euo pipefail
umask 077

FORCE=0
if [[ $# -eq 1 && "$1" == "--force" ]]; then
  FORCE=1
elif [[ $# -ne 0 ]]; then
  printf 'Использование: scripts/mattpocock-skills-update.sh [--force]\n' >&2
  exit 2
fi

export MATTPOCOCK_UPDATE_SCRIPT="${BASH_SOURCE[0]}"
export MATTPOCOCK_UPDATE_FORCE="$FORCE"

python3 -I - <<'PY'
import datetime
import fcntl
import json
import os
import pathlib
import re
import shutil
import stat
import subprocess
import sys
import tempfile

REPO_URL = "https://github.com/mattpocock/skills.git"
SKILLS_SOURCE = "mattpocock/skills"
SKILLS_CLI = "skills@1.7.4"


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def ensure_safe_dir(path, mode=0o700):
    path.mkdir(parents=True, exist_ok=True, mode=mode)
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode):
        raise RuntimeError(f"Refusing symlink directory: {path}")
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid():
        raise RuntimeError(f"Unsafe directory: {path}")
    if info.st_mode & 0o022:
        raise RuntimeError(f"Directory is writable by others: {path}")
    if mode == 0o700 and (info.st_mode & 0o077):
        os.chmod(path, 0o700)


def is_safe_name(name):
    return isinstance(name, str) and bool(name) and "/" not in name and "\\" not in name and not name.startswith(".")


def read_private_json(path):
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise RuntimeError(f"Unsafe state file: {path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise RuntimeError(f"Expected JSON object in {path}")
    return data


def write_private_json(path, data):
    fd, tmp_name = tempfile.mkstemp(prefix=".mattpocock-skills-", dir=path.parent)
    tmp_path = pathlib.Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            stream.write(json.dumps(data, indent=2, ensure_ascii=False) + "\n")
        tmp_path.chmod(0o600)
        tmp_path.replace(path)
    finally:
        tmp_path.unlink(missing_ok=True)


def fetch_remote_head():
    env = dict(os.environ, GIT_TERMINAL_PROMPT="0")
    result = subprocess.run(
        ["git", "ls-remote", "--exit-code", REPO_URL, "refs/heads/main"],
        env=env,
        stdin=subprocess.DEVNULL,
        capture_output=True,
        text=True,
        timeout=30,
    )
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit {result.returncode}"
        raise RuntimeError(f"git ls-remote failed: {detail}")
    first_line = result.stdout.strip().splitlines()[0] if result.stdout.strip() else ""
    sha = first_line.split()[0] if first_line.split() else ""
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise RuntimeError(f"Unexpected git ls-remote output: {first_line!r}")
    return sha


def read_lock_skills(lock_path):
    try:
        if not lock_path.is_file() or lock_path.is_symlink():
            return None
        data = json.loads(lock_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError, UnicodeDecodeError):
        return None
    skills = data.get("skills") if isinstance(data, dict) else None
    return skills if isinstance(skills, dict) else None


def parse_install_results(raw_stdout, agents_skills):
    try:
        payload = json.loads(raw_stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"Unexpected non-JSON output from {SKILLS_CLI}: {exc}") from exc
    if not isinstance(payload, list) or not payload:
        raise RuntimeError(f"Empty or invalid install result from {SKILLS_CLI}")

    resolved_base = agents_skills.resolve(strict=True)
    installed = []
    for entry in payload:
        if not isinstance(entry, dict):
            raise RuntimeError(f"Malformed install entry from {SKILLS_CLI}")
        if entry.get("status") != "installed":
            name = entry.get("name", "<unknown>")
            err = entry.get("error") or entry.get("status") or "failed"
            raise RuntimeError(f"Skill {name!r} failed to install: {err}")
        raw_path = entry.get("path")
        if not isinstance(raw_path, str) or not raw_path:
            raise RuntimeError(f"Missing install path for {entry.get('name', '<unknown>')!r}")
        skill_dir = pathlib.Path(os.path.expanduser(raw_path)).resolve(strict=True)
        if skill_dir.parent != resolved_base or not is_safe_name(skill_dir.name) or not (skill_dir / "SKILL.md").is_file():
            raise RuntimeError(f"Unexpected install path for {entry.get('name', '<unknown>')!r}: {skill_dir}")
        installed.append(skill_dir.name)
    return sorted(set(installed))


def skills_present(names, agents_skills, claude_profile, gemini_dir, agy_plugin_skills):
    if not names:
        return False
    check_claude = claude_profile.is_dir() and not claude_profile.is_symlink()
    check_agy = gemini_dir.is_dir() and not gemini_dir.is_symlink()
    for name in names:
        if not is_safe_name(name):
            return False
        if not (agents_skills / name / "SKILL.md").is_file():
            return False
        if check_claude and not (claude_profile / "skills" / name / "SKILL.md").is_file():
            return False
        if check_agy and not (agy_plugin_skills / name / "SKILL.md").is_file():
            return False
    return True


def main():
    # Resolve through ~/.config/dots symlink to locate sibling sync scripts in dots.
    script_path = pathlib.Path(os.environ["MATTPOCOCK_UPDATE_SCRIPT"]).resolve(strict=True)
    repo = script_path.parent.parent
    claude_script = repo / "scripts/claude-config.sh"
    agy_script = repo / "scripts/agy-setup.sh"
    if not claude_script.is_file() or not agy_script.is_file():
        raise RuntimeError(f"Sync scripts not found under {repo}")

    home = pathlib.Path.home().resolve(strict=True)
    state_dir = home / ".local/state/dots"
    ensure_safe_dir(state_dir, 0o700)

    lock_file = state_dir / "mattpocock-skills-update.lock"
    lock_fd = os.open(lock_file, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        lock_info = os.fstat(lock_fd)
        if not stat.S_ISREG(lock_info.st_mode) or lock_info.st_uid != os.getuid():
            raise RuntimeError(f"Unsafe lock file: {lock_file}")
        try:
            fcntl.flock(lock_fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return

        force = os.environ.get("MATTPOCOCK_UPDATE_FORCE") == "1"
        state_path = state_dir / "mattpocock-skills.json"
        state = read_private_json(state_path) if state_path.exists() or state_path.is_symlink() else {}
        prev_skills = [s for s in state.get("skills", []) if is_safe_name(s)] if isinstance(state.get("skills"), list) else []

        agents_skills = home / ".agents/skills"
        claude_profile = home / ".local/share/claude-guard/home/.claude"
        gemini_dir = home / ".gemini"
        agy_plugin_skills = gemini_dir / "config/plugins/migrated-settings/skills"

        remote_sha = fetch_remote_head()
        if not force and state.get("commit") == remote_sha and skills_present(prev_skills, agents_skills, claude_profile, gemini_dir, agy_plugin_skills):
            if sys.stdout.isatty():
                print(f"mattpocock-skills: уже актуально ({remote_sha[:7]}, {len(prev_skills)} скиллов)")
            return

        # Keep skills@1.7.4 anchored to ~/.agents/.skill-lock.json regardless of shell XDG_STATE_HOME.
        env = dict(os.environ, DISABLE_TELEMETRY="1", GIT_TERMINAL_PROMPT="0")
        env.pop("XDG_STATE_HOME", None)
        install = subprocess.run(
            [
                "npx", "--yes", SKILLS_CLI, "add",
                "https://github.com/mattpocock/skills",
                "--global", "--agent", "universal", "--skill", "*", "--yes", "--json",
            ],
            env=env,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            text=True,
            timeout=300,
        )
        if install.returncode != 0:
            detail = install.stderr.strip() or install.stdout.strip() or f"exit {install.returncode}"
            raise RuntimeError(f"skills add failed: {detail}")

        current_skills = parse_install_results(install.stdout, agents_skills)

        removed = []
        if prev_skills:
            lock_skills = read_lock_skills(home / ".agents/.skill-lock.json")
            if lock_skills is not None:
                for name in sorted(set(prev_skills) - set(current_skills)):
                    meta = lock_skills.get(name)
                    if not isinstance(meta, dict) or meta.get("source") != SKILLS_SOURCE:
                        continue
                    agents_target = agents_skills / name
                    if agents_target.exists() and not agents_target.is_symlink() and agents_target.is_dir():
                        shutil.rmtree(agents_target)
                        removed.append(name)
                    cg_link = claude_profile / "skills" / name
                    if cg_link.is_symlink() and os.readlink(cg_link) == str(agents_target):
                        cg_link.unlink()

        if claude_profile.is_dir() and not claude_profile.is_symlink():
            res = subprocess.run(
                ["/bin/bash", str(claude_script)],
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                timeout=60,
            )
            if res.returncode != 0:
                detail = res.stderr.strip() or res.stdout.strip() or f"exit {res.returncode}"
                raise RuntimeError(f"claude-config.sh failed: {detail}")

        if gemini_dir.is_dir() and not gemini_dir.is_symlink():
            res = subprocess.run(
                ["/bin/bash", str(agy_script)],
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                timeout=120,
            )
            if res.returncode != 0:
                detail = res.stderr.strip() or res.stdout.strip() or f"exit {res.returncode}"
                raise RuntimeError(f"agy-setup.sh failed: {detail}")

        write_private_json(state_path, {"commit": remote_sha, "skills": current_skills})
        extra = f", удалено: {len(removed)}" if removed else ""
        print(f"{utc_now()} mattpocock-skills: обновлено до {remote_sha[:7]} ({len(current_skills)} скиллов{extra})")
    finally:
        os.close(lock_fd)


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as exc:
        print(f"{utc_now()} mattpocock-skills: {exc}", file=sys.stderr)
        sys.exit(1)
PY
