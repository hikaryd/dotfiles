# BMAD Codex

Reply in Russian unless the user requests another language.

Use the installed BMAD workflows for product planning, architecture, implementation
and review. The `bmad` skill is the entrypoint for setup and choosing the next
workflow. Module records named `bmod-*` describe installed modules.

Skills live in `$CODEX_HOME/skills`. Pass this folder as `--root` to BMAD setup
and knowledge scripts. Read the selected skill's `SKILL.md` and follow its actual
workflow. Run its Python scripts through `uv`.

BMAD runtime belongs to the working project: `_bmad/` contains configuration and
scripts, and `_bmad-output/` contains generated artifacts. When a requested BMAD
workflow needs a runtime that is absent, initialize it with `codex-bmad init`
in the project, then continue. Existing repository instructions and the user's
artifact destinations apply to the work.
