#!/usr/bin/env python3
"""Install pinned BMAD skills into an independent Codex home."""

import argparse
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import sys
import tempfile
import tomllib

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from codex_config import _atomic_write, validate_with_codex

ABIDE = runpy.run_path(str(ROOT / "scripts/abide-apply.py"))
SOURCE = json.loads((ROOT / "config/codex-bmad/source.json").read_text())


def write(path, content):
    if path.is_symlink():
        raise ValueError(f"expected a regular file: {path}")
    if not path.exists() or path.read_text() != content:
        _atomic_write(path, content, 0o600)


def link(target, source):
    if target.is_symlink() and target.resolve() == source.resolve():
        return
    if target.exists() or target.is_symlink():
        raise ValueError(f"existing file preserved; cannot create link: {target}")
    target.parent.mkdir(parents=True, exist_ok=True)
    target.symlink_to(source)


def install_skills(directory, primary):
    target = directory / "skills"
    record = directory / "bmad-source.json"
    installed = json.loads(record.read_text()) if record.exists() else None
    if installed is not None and installed != SOURCE:
        raise ValueError("BMAD source pin changed; installed skills preserved. Use a new CODEX_BMAD_HOME.")
    missing = [name for name in SOURCE["skills"] if not (target / name).exists()]
    if missing:
        helper = primary / "skills/.system/skill-installer/scripts/install-skill-from-github.py"
        if not helper.is_file():
            raise ValueError(f"Codex skill-installer is missing: {helper}; start the primary Codex once")
        # Download once, validate every skill, then publish the complete batch.
        with tempfile.TemporaryDirectory(prefix="bmad-skills-") as temporary:
            staging = Path(temporary) / "skills"
            subprocess.run([
                sys.executable, str(helper), "--repo", SOURCE["repo"], "--ref", SOURCE["ref"],
                "--dest", str(staging), "--path", *[f"skills/{name}" for name in missing],
            ], check=True)
            target.mkdir(parents=True, exist_ok=True)
            for name in missing:
                if not (staging / name / "SKILL.md").is_file():
                    raise ValueError(f"BMAD skill payload is incomplete: {name}")
            for name in missing:
                shutil.move(str(staging / name), target / name)
    for name in SOURCE["skills"]:
        if not (target / name / "SKILL.md").is_file():
            raise ValueError(f"existing BMAD skill is incomplete; preserved: {target / name}")
    write(record, json.dumps(SOURCE, indent=2) + "\n")


def install(directory, primary):
    binary = shutil.which("codex")
    if binary is None or shutil.which("uv") is None:
        raise ValueError("codex and uv must be installed before installing BMAD")
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    install_skills(directory, primary)
    config = directory / "config.toml"
    if not config.exists():
        live = tomllib.loads((primary / "config.toml").read_text()) if (primary / "config.toml").exists() else {}
        settings = "".join(
            f"{key} = {json.dumps(live[key])}\n"
            for key in ("model", "model_reasoning_effort", "service_tier") if key in live
        )
        content = settings + (ROOT / "config/codex-bmad/config.toml").read_text()
        validate_with_codex(content, Path(binary))
        write(config, content)
    if (primary / "auth.json").is_file() and not (directory / "auth.json").exists():
        link(directory / "auth.json", primary / "auth.json")

    # CODEX_HOME isolates legacy user skills; ~/.agents skills need explicit overrides.
    profile = "[skills]\nmax_context_tokens = 10000\n"
    for skill in sorted((Path.home() / ".agents/skills").glob("*/SKILL.md")):
        if skill.parent.name == "abide-compile":
            continue
        profile += f"\n[[skills.config]]\npath = {json.dumps(str(skill))}\nenabled = false\n"
    # Toolsmith embeds a sample SKILL.md as an asset; retain it without activating it.
    for name in SOURCE["skills"]:
        folder = directory / "skills" / name
        for skill in sorted(folder.rglob("SKILL.md")):
            if skill.parent != folder:
                profile += f"\n[[skills.config]]\npath = {json.dumps(str(skill))}\nenabled = false\n"
    begin, end = "# BEGIN DOTS BMAD SKILLS", "# END DOTS BMAD SKILLS"
    current = config.read_text()
    if begin in current:
        before, rest = current.split(begin, 1)
        _, after = rest.split(end, 1)
        unmanaged = before + after.lstrip("\n")
    else:
        unmanaged = current
    if "skills" in tomllib.loads(unmanaged):
        raise ValueError("existing unmanaged skills settings preserved; merge them before reapplying BMAD")
    unmanaged = ABIDE["replace_table"](unmanaged, "shell_environment_policy.set", {
        "CODEX_HOME": str(directory), "CODEX_BMAD_HOME": str(directory),
    })
    updated = unmanaged.rstrip() + f"\n\n{begin}\n{profile}{end}\n"
    validate_with_codex(updated, Path(binary))
    write(config, updated)

    autonomy = (
        "<!-- AUTONOMY DIRECTIVE — DO NOT REMOVE -->\n"
        "YOU ARE AN AUTONOMOUS CODING AGENT. EXECUTE TASKS TO COMPLETION WITHOUT ASKING FOR PERMISSION.\n"
        'DO NOT STOP TO ASK "SHOULD I PROCEED?" — PROCEED. DO NOT WAIT FOR CONFIRMATION ON OBVIOUS NEXT STEPS.\n'
        "IF BLOCKED, TRY AN ALTERNATIVE APPROACH. ONLY ASK WHEN TRULY AMBIGUOUS OR DESTRUCTIVE.\n"
        "USE CODEX NATIVE SUBAGENTS FOR INDEPENDENT PARALLEL SUBTASKS WHEN THAT IMPROVES THROUGHPUT.\n"
        "<!-- END AUTONOMY DIRECTIVE -->\n\n"
    )
    instructions = autonomy + (ROOT / "config/codex-bmad/instructions.md").read_text()
    for marker, source in (
        ("DOTS SEARCH", ROOT / "config/codex/search-instructions.md"),
        ("DOTS ABIDE", ROOT / "config/abide/instructions.md"),
    ):
        instructions += f"\n<!-- BEGIN {marker} -->\n{source.read_text().rstrip()}\n<!-- END {marker} -->\n"
    write(directory / "AGENTS.md", instructions)
    ABIDE["configure_hooks"](directory / "hooks.json")
    ABIDE["configure_codex_trust"](directory)
    launcher = ROOT / "scripts/codex-bmad"
    launcher.chmod(0o755)
    link(Path.home() / ".local/bin/codex-bmad", launcher)
    print(f"codex-bmad: {len(SOURCE['skills'])} skills from {SOURCE['version']} ({SOURCE['ref'][:12]}) in {directory}")
    if not (directory / "auth.json").exists():
        print("codex-bmad: run codex-bmad login to authenticate this instance")


def init_project(directory, project):
    if not project.is_dir():
        raise ValueError(f"project directory does not exist: {project}")
    skill = directory / "skills/bmad"
    result = subprocess.run([
        "uv", "run", "--no-cache", str(skill / "scripts/setup.py"),
        "--project-root", str(project), "--skill", str(skill), "--root", str(directory / "skills"),
    ], check=True, stdout=subprocess.PIPE, text=True)
    report = json.loads(result.stdout)
    if not report.get("current"):
        print(result.stdout, end="")
        raise ValueError("BMAD setup reported unresolved installation problems")
    print(f"codex-bmad: runtime ready in {project / '_bmad'}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--init-project", type=Path)
    args = parser.parse_args()
    directory = Path(os.environ.get("CODEX_BMAD_HOME", str(Path.home() / ".codex-bmad"))).expanduser().resolve()
    primary = Path(os.environ.get("CODEX_BMAD_PRIMARY_HOME", str(Path.home() / ".codex"))).expanduser().resolve()
    if (
        directory.is_relative_to(ROOT) or ROOT.is_relative_to(directory)
        or directory.is_relative_to(primary) or primary.is_relative_to(directory)
    ):
        raise ValueError("BMAD home must be separate from primary Codex and outside the dotfiles repository")
    if args.init_project:
        init_project(directory, args.init_project.expanduser().resolve())
    else:
        install(directory, primary)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        print(f"codex-bmad: {exc}", file=sys.stderr)
        sys.exit(1)
