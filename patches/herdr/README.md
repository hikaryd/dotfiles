# Automatic centered, rounded Herdr tabs

Tab alignment and rounded caps are a local client-rendering patch, not a theme
setting. `scripts/herdr` automatically ensures a patched client when attaching a
UI (`herdr`, named sessions, or remote attach).

After a Homebrew update, the first attach builds the **new installed version**.
Changing the patch also invalidates an existing same-version build. Compilation
can take several minutes; later attaches reuse the verified client. Metadata,
server-management and update commands do not build anything and use Homebrew.
The wrapper resolves its own symlink in `~/.local/bin/herdr`.

## Safety and compatibility

- Reviewed commits are pinned for **0.9.1** and **0.9.4**.
- New stable versions fetch their exact upstream release tag and try the
  maintained rendering patch. No older client is forced onto a newer server.
- Before installation, the patch must apply cleanly, the rounded/centered tab
  regression test must actually run and pass, and the built binary's version
  must match the requested release. The source commit is recorded.
- Installation is atomic. Concurrent attaches share one build; dead process
  locks are reclaimed automatically.
- On incompatibility, missing dependencies or network/build failure, the
  wrapper warns and attaches with the last good **same-version** client or the
  standard Homebrew client. An incompatible upstream change still needs a patch
  update; the script never rewrites the patch automatically.
- Failed automatic attempts are throttled for 15 minutes. A changed patch or a
  manual build retries immediately.
- Neither the build nor fallback stops a server or its panes.

## Dependencies and cache

Cargo/Rust and macOS developer tools must be installed. If needed:

```sh
brew install rust
```

The build uses **Zig 0.16.0**. If Homebrew has a different Zig version, the script
downloads the appropriate macOS compiler from `ziglang.org`, verifies its pinned
SHA-256 **before extraction**, and stores it in the private cache. It does not
replace global Zig. An explicit `ZIG=/path/to/zig` override must also be 0.16.0.

Clients, patch fingerprints, source commits and `build.log` live under
`${XDG_DATA_HOME:-~/.local/share}/dots/herdr/<version>/`. The Zig toolchain and
shared Cargo build cache live under `${XDG_CACHE_HOME:-~/.cache}/dots/herdr/`.

## Retry, opt out and test

```sh
# Force a retry for the currently installed release, bypassing failure cooldown.
scripts/build-herdr-ui.sh

# Use the original Homebrew client for this invocation, with no build attempt.
HERDR_DOTS_NO_PATCH=1 herdr

# Isolated fake-runtime contracts: caching, upgrades, failures and concurrent starts.
bash tests/test-herdr-auto-patch.sh

# Real rendering check in a temporary PTY/server, after the build is cached.
uv run --with pyte python tests/test-herdr-ui.py scripts/herdr
```

The contract suite is also included in `scripts/test.sh runner`.

An already attached client keeps its existing renderer. To pick up a newly built
client, detach with **Ctrl+A, then D** and run `herdr` again; patch selection and
any required build are automatic. Do not run `herdr server stop`: the persistent
server and its panes do not need restarting.
