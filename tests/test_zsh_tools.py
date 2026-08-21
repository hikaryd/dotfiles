#!/usr/bin/env python3

"""Regression checks for lightweight interactive Zsh integrations."""

from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
ZDOTDIR = REPO_ROOT / "config/zsh"


def main() -> None:
    zsh = "/opt/homebrew/bin/zsh"
    with tempfile.TemporaryDirectory(prefix="dots-zsh-tools-") as temporary_home:
        home = Path(temporary_home)
        (home / "dots").symlink_to(REPO_ROOT, target_is_directory=True)
        (home / ".cargo").mkdir()
        (home / ".cargo" / "env").touch()
        (home / ".config/zsh").mkdir(parents=True)
        (home / ".config/zsh/functions").symlink_to(
            ZDOTDIR / "functions", target_is_directory=True
        )

        fake_bin = home / "fake-bin"
        fake_bin.mkdir()
        target = home / "selected-directory"
        target.mkdir()
        fake_yazi = fake_bin / "yazi"
        fake_yazi.write_text(
            "#!/bin/sh\n"
            "for arg in \"$@\"; do\n"
            "  case $arg in --cwd-file=*) printf '%s\\n' \"$YAZI_TEST_CWD\" > \"${arg#*=}\";; esac\n"
            "done\n"
        )
        fake_yazi.chmod(0o755)

        environment = os.environ | {
            "HOME": str(home),
            "ZDOTDIR": str(ZDOTDIR),
            "TERM": "xterm-256color",
            "YAZI_TEST_CWD": str(target),
        }
        script = f"""
          [[ $widgets[edit-command-line] == user:* ]]
          [[ $(bindkey '^[e') == *edit-command-line* ]]
          [[ $(whence -w z) == 'z: function' ]]
          [[ $(whence -w zi) == 'zi: function' ]]
          _dots-zoxide-init
          (( $+functions[__zoxide_z] ))
          (( ${{chpwd_functions[(Ie)__zoxide_hook]}} ))
          [[ $ZSH_AUTOSUGGEST_STRATEGY == history ]]
          [[ $ZSH_AUTOSUGGEST_BUFFER_MAX_SIZE == 256 ]]
          [[ $ZSH_HIGHLIGHT_MAXLENGTH == 512 ]]
          (( $+ZSH_AUTOSUGGEST_MANUAL_REBIND ))
          [[ $OMX_NOTIFY_FALLBACK == 0 ]]
          (( ${{precmd_functions[(Ie)_dots-prompt-precmd]}} ))
          (( ${{preexec_functions[(Ie)_dots-prompt-preexec]}} ))
          [[ $PROMPT == *'󰀵'* ]]
          (( ! $+functions[prompt_starship_precmd] ))
          (( $+widgets[fzf-tab-complete] ))
          zstyle -a ':fzf-tab:*' fzf-flags fzf_tab_flags
          [[ "${{(j: :)fzf_tab_flags}}" == *'ctrl-j:down,ctrl-k:up,ctrl-c:abort,tab:accept'* ]]
          [[ $(whence -w y) == 'y: function' ]]
          path=({fake_bin!s} $path)
          rehash
          y
          [[ $PWD == {target!s} ]]
        """
        completed = subprocess.run(
            [zsh, "-d", "-i", "-c", script],
            env=environment,
            cwd=REPO_ROOT,
            text=True,
            capture_output=True,
            timeout=20,
        )
        if completed.returncode:
            raise AssertionError(
                f"zsh integration check failed ({completed.returncode})\n"
                f"stdout:\n{completed.stdout}\nstderr:\n{completed.stderr}"
            )

    print("ok: zoxide, fzf-tab, Alt+E, and Yazi cwd handoff")


if __name__ == "__main__":
    main()
