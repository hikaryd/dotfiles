#!/usr/bin/env python3
"""Merge the managed search instructions without replacing other global rules."""
import os
from pathlib import Path
import shutil
import time

ROOT = Path(__file__).resolve().parent.parent
BEGIN = '<!-- BEGIN DOTS SEARCH -->'
END = '<!-- END DOTS SEARCH -->'


def main():
    base = Path(os.environ.get('CODEX_CONFIG_DIR') or os.environ.get('CODEX_HOME') or Path.home() / '.codex')
    target = base / 'AGENTS.md'
    original = target.read_text() if target.exists() else ''
    block = BEGIN + '\n' + (ROOT / 'config/codex/search-instructions.md').read_text().rstrip() + '\n' + END
    if BEGIN in original:
        before, rest = original.split(BEGIN, 1)
        _, after = rest.split(END, 1)
        updated = before + block + after
    else:
        updated = original.rstrip() + '\n\n' + block + '\n'
    if updated == original:
        print('Codex search instructions unchanged')
        return
    base.mkdir(parents=True, exist_ok=True)
    if target.exists():
        shutil.copy2(target, target.with_name(f'AGENTS.md.bak-{time.time_ns()}'))
    target.write_text(updated)
    print(f'Codex search instructions installed: {target}')


if __name__ == '__main__':
    main()
