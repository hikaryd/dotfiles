## Repository search

Read a named file (for example `ARCHITECTURE.md`) directly. Use `rg` for known paths, symbols, and literal strings. Use JevGrep for a behavior or concept whose location is unknown.

For semantic search, call the MCP tool `semantic_search_code` directly when it is available. That name is an MCP tool, not a `jevgrep` CLI subcommand. Choose a plausible narrow `scope` when one is known. Do not run `--help`, `doctor`, or `inspect` before a normal search; do not run duplicate searches while the first is silent. Read the returned source before editing. Partial or empty results do not establish absence.

If the MCP tool is unavailable, run the CLI directly: `~/dots/scripts/jevgrep-codex search --query "<question>" --scope <directory>` from the repository root. If no directory is known, omit `--scope`. If the CLI says the repository has no profile, run `~/dots/scripts/jevgrep-codex init --root <absolute-repository-root>` once and retry the search once. The user authorizes initialization and remote evaluation of eligible source for a repository they ask you to work on. For a newly created profile, verify its `repository_root` and enable `remote_evaluation_enabled` in the external profile printed by init. Respect repository-specific disclosure restrictions; preserve explicitly disabled existing profiles and never invent credentials or switch providers. A new profile needs a new Codex session before its MCP server can start.

If the search fails or exceeds its scope, report the cause and use local `rg` to continue. Use `doctor` or `inspect` only to diagnose a specific error or estimate a costly broad scan. Do not blind-retry paid requests.
