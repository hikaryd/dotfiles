# Codex config

Personal [Codex](https://developers.openai.com/codex/) configuration, versioned
without credentials or runtime state.

Codex keeps both durable preferences and mutable state in `~/.codex/config.toml`.
For that reason this directory is **not** symlinked over `~/.codex`: the apply
script merges the tracked subset and preserves the local-only sections.

## What is tracked

| File | Destination | Notes |
|------|-------------|-------|
| `config.toml` | `~/.codex/config.toml` | Stable model/provider, shell policy, MCP, agent, and TUI settings |
| `rules/dots.rules` | `$CODEX_HOME/rules/dots.rules` | Curated portable approval rules; installed as a symlink |
| `mcp-secrets.example.sh` | template for `~/.codex/.mcp-secrets` | Real values stay outside git |

`scripts/codex-apply.sh` owns only the stable keys represented in the template.
It preserves project trust, hook trust hashes, plugins/marketplaces, OMX feature
state, NUX state, and generated `developer_instructions` already present in the
live config.

The tracked template uses placeholders for `Z_AI_API_KEY` and the private
Cloudflare gateway URL. Real values must live in `~/.codex/.mcp-secrets` with
mode `0600`.

## Restore on a new machine

```bash
# 1. Install/regenerate Codex and OMX-owned infrastructure.
omx setup --scope user --plugin

# 2. Add private values once.
mkdir -p ~/.codex
cp ~/dots/config/codex/mcp-secrets.example.sh ~/.codex/.mcp-secrets
chmod 600 ~/.codex/.mcp-secrets
$EDITOR ~/.codex/.mcp-secrets

# 3. Merge the personal snapshot.
cd ~/dots && ./install -c steps/codex.yml
```

The apply step is idempotent, writes `config.toml` atomically with mode `0600`,
and creates a private (`0600`) `config.toml.bak-<timestamp>` before a change. If
a secret is absent, an already configured secret-backed MCP/provider block is
preserved; on a fresh machine that block is skipped. Managed root/table markers
record which values were actually applied so uninstall never guesses ownership.
Close active Codex sessions before apply/install as well as sync; the scripts
detect intervening writes and abort rather than overwriting a concurrently
changed config.

Codex also loads the separate tracked `dots.rules`. Local approvals learned by
Codex remain in `~/.codex/rules/default.rules` and are neither overwritten nor
exported: that file can contain machine-specific paths and overly broad rules.
The installer validates the tracked policy with `codex execpolicy check`.
Edit portable rules directly in `config/codex/rules/dots.rules`; do not sync the
learned file wholesale. Rules are still an experimental Codex surface, so the
validation step is intentionally part of every install.

## Update dotfiles after changing Codex

Close active Codex sessions first so they cannot write UI/hook state during the
export, then run:

```bash
~/dots/scripts/codex-sync.sh
git -C ~/dots add -p config/codex/
```

The sync script exports only the managed subset, replaces secrets with
`${...}` placeholders, removes absolute home paths and comments, validates TOML,
and fails closed if static HTTP headers, credential-bearing arguments/URLs, or
runtime-owned data could reach the snapshot.

`./uninstall` first copies the complete live config into its timestamped backup,
then removes only entries carrying apply-owned markers. Project trust, hook/plugin
state, locally added MCP servers, and secret-backed groups that apply merely
preserved remain active.

## Intentionally not versioned

- `auth.json`, `.env`, credentials, proxy values, OAuth state.
- `projects`, `hooks.state`, NUX/notice state, local plugin/marketplace paths.
- `AGENTS.md`, `agents/`, OMX skills/hooks, `.omx/`, and plugin cache — owned by
  the chosen OMX/plugin setup mode, not this snapshot. Re-run that setup
  explicitly; plugin mode can preserve an existing generated `AGENTS.md`.
- `hooks.json` and `herdr-agent-state.sh` — currently managed by Muxy/Herdr and
  must not be frozen by dotfiles; reconfigure those integrations separately.
- Codex-learned `rules/default.rules`, sessions, history, memories/goals, logs,
  shell snapshots, caches, and SQLite databases.
- Standalone personal skills (for example `full-regression`) are separate
  workflow sources and are not restored by this config snapshot.

The shell aliases in this repository can still override values from TOML (for
example the CLI reasoning effort), which is expected Codex precedence behavior.
