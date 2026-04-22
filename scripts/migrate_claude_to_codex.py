#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
import shutil
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path


@dataclass(frozen=True)
class CopyItem:
    source: Path
    destination: Path
    kind: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Import Claude skills and agents into Codex."
    )
    parser.add_argument(
        "--claude-dir",
        default=str(Path.home() / ".claude"),
        help="Path to ~/.claude",
    )
    parser.add_argument(
        "--codex-dir",
        default=str(Path.home() / ".codex"),
        help="Path to ~/.codex",
    )
    parser.add_argument(
        "--apply",
        action="store_true",
        help="Write changes. Without this flag the script only prints the plan.",
    )
    return parser.parse_args()


def rewrite_skill_frontmatter(skill_md: Path, new_name: str) -> None:
    text = skill_md.read_text(encoding="utf-8")
    if text.startswith("---\n"):
        parts = text.split("\n---\n", 1)
        if len(parts) == 2:
            front, body = parts
            if re.search(r"(?m)^name:\s*.*$", front):
                front = re.sub(
                    r"(?m)^name:\s*.*$",
                    f"name: {new_name}",
                    front,
                    count=1,
                )
            else:
                front = front.rstrip() + f"\nname: {new_name}"
            skill_md.write_text(front + "\n---\n" + body, encoding="utf-8")
            return

    skill_md.write_text(
        f"---\nname: {new_name}\ndescription: Imported Claude skill {new_name}.\n---\n\n"
        + text,
        encoding="utf-8",
    )


def build_agent_wrapper(source_md: Path, destination_md: Path, wrapper_name: str) -> None:
    text = source_md.read_text(encoding="utf-8")
    description = f"Imported Claude agent wrapper for {source_md.stem}."
    body = text

    if text.startswith("---\n"):
        parts = text.split("\n---\n", 1)
        if len(parts) == 2:
            front, body = parts
            match = re.search(r"(?m)^description:\s*(.+)$", front)
            if match:
                description = match.group(1).strip().strip('"')

    wrapper = (
        f"---\nname: {wrapper_name}\ndescription: {description}\n---\n\n"
        f"> Imported from `~/.claude/agents/{source_md.name}` and exposed as a Codex skill wrapper.\n\n"
        + body.lstrip()
    )
    destination_md.write_text(wrapper, encoding="utf-8")


def copy_path(source: Path, destination: Path) -> None:
    if source.is_dir():
        shutil.copytree(
            source,
            destination,
            dirs_exist_ok=False,
            ignore=shutil.ignore_patterns(".DS_Store", "__pycache__"),
        )
    else:
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)


def backup_if_exists(path: Path, backup_root: Path) -> None:
    if not path.exists():
        return
    backup_root.mkdir(parents=True, exist_ok=True)
    target = backup_root / path.name
    if target.exists():
        if target.is_dir():
            shutil.rmtree(target)
        else:
            target.unlink()
    shutil.move(str(path), str(target))


def ensure_exists(path: Path, label: str) -> None:
    if not path.exists():
        raise SystemExit(f"Missing {label}: {path}")


def collect_reference_items(claude_dir: Path, codex_dir: Path) -> list[CopyItem]:
    ref_root = codex_dir / "vendor_imports" / "claude-reference"
    candidates = [
        claude_dir / "AGENTS.md",
        claude_dir / "settings.json",
        claude_dir / "plugin.json",
        claude_dir / "marketplace.json",
        claude_dir / "commands",
        claude_dir / "rules",
        claude_dir / "hooks",
        claude_dir / ".agents" / "plugins",
    ]

    items: list[CopyItem] = []
    for src in candidates:
        if src.exists():
            items.append(CopyItem(src, ref_root / src.name, "reference"))
    return items


def main() -> int:
    args = parse_args()
    claude_dir = Path(args.claude_dir).expanduser().resolve()
    codex_dir = Path(args.codex_dir).expanduser().resolve()

    claude_skills = claude_dir / ".agents" / "skills"
    claude_agents = claude_dir / "agents"
    codex_skills = codex_dir / "skills"
    backup_root = (
        codex_dir
        / "import-backups"
        / f"claude-migration-{datetime.now().strftime('%Y%m%d-%H%M%S')}"
    )

    ensure_exists(claude_dir, "Claude directory")
    ensure_exists(codex_dir, "Codex directory")
    ensure_exists(claude_skills, "Claude skills directory")
    ensure_exists(claude_agents, "Claude agents directory")
    ensure_exists(codex_skills, "Codex skills directory")

    skill_items = [
        CopyItem(src, codex_skills / f"claude-{src.name}", "skill")
        for src in sorted(claude_skills.iterdir())
        if src.is_dir()
    ]
    agent_items = [
        CopyItem(src, codex_skills / f"claude-agent-{src.stem}", "agent-wrapper")
        for src in sorted(claude_agents.glob("*.md"))
    ]
    reference_items = collect_reference_items(claude_dir, codex_dir)

    plan = {
        "claude_dir": str(claude_dir),
        "codex_dir": str(codex_dir),
        "skills_to_import": len(skill_items),
        "agents_to_wrap": len(agent_items),
        "reference_items_to_copy": len(reference_items),
        "sample_skills": [item.destination.name for item in skill_items[:8]],
        "sample_agents": [item.destination.name for item in agent_items[:8]],
    }

    print(json.dumps(plan, indent=2))

    if not args.apply:
        print("\nDry run only. Re-run with --apply to perform the migration.")
        return 0

    manifest = {
        "imported_at": datetime.now().isoformat(),
        "source": str(claude_dir),
        "skills": [],
        "agent_wrappers": [],
        "references": [],
    }

    for item in skill_items:
        backup_if_exists(item.destination, backup_root)
        copy_path(item.source, item.destination)
        skill_md = item.destination / "SKILL.md"
        if skill_md.exists():
            rewrite_skill_frontmatter(skill_md, item.destination.name)
        manifest["skills"].append(
            {"source": str(item.source), "destination": str(item.destination)}
        )

    for item in agent_items:
        backup_if_exists(item.destination, backup_root)
        item.destination.mkdir(parents=True, exist_ok=True)
        build_agent_wrapper(
            item.source,
            item.destination / "SKILL.md",
            item.destination.name,
        )
        manifest["agent_wrappers"].append(
            {
                "source": str(item.source),
                "destination": str(item.destination / "SKILL.md"),
            }
        )

    for item in reference_items:
        backup_if_exists(item.destination, backup_root)
        copy_path(item.source, item.destination)
        manifest["references"].append(
            {"source": str(item.source), "destination": str(item.destination)}
        )

    ref_root = codex_dir / "vendor_imports" / "claude-reference"
    ref_root.mkdir(parents=True, exist_ok=True)
    (ref_root / "README.md").write_text(
        "# Claude Reference Import\n\n"
        "This folder contains settings and auxiliary files copied from `~/.claude`.\n\n"
        "- These files were copied for reference only.\n"
        "- They are not automatically activated by Codex.\n"
        "- Imported skills live in `~/.codex/skills/claude-*`.\n"
        "- Imported agent wrappers live in `~/.codex/skills/claude-agent-*`.\n",
        encoding="utf-8",
    )
    manifest_path = ref_root / "import-manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2), encoding="utf-8")

    print(
        json.dumps(
            {
                "backup_root": str(backup_root),
                "skills_imported": len(manifest["skills"]),
                "agent_wrappers_created": len(manifest["agent_wrappers"]),
                "reference_items_copied": len(manifest["references"]),
                "manifest": str(manifest_path),
            },
            indent=2,
        )
    )
    print("\nMigration complete. Restart Codex to pick up the new skills.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
