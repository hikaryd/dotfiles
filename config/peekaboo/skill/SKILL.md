---
name: peekaboo
description: Control macOS desktop apps through Peekaboo MCP. Use for screenshots, UI inspection, clicks, typing and window/menu actions on this Mac, or when Peekaboo reports dispatched_unverified.
---

# Peekaboo: observe → act → verify

Use the `peekaboo` MCP server in Jcode. Discover its tools with native
`mcp_search` filtered by server `peekaboo`; inspect schemas before calling.
Desktop tools are local. Built-in `agent`, `analyze` and `browser` tools are excluded;
the calling agent supplies reasoning and vision without extra Peekaboo API keys.

1. Check `permissions` when starting desktop work or diagnosing a permission error.
   Screen Recording and Accessibility must be granted; synthetic input also needs
   Event Synthesizing. Missing grants require the human's System Settings action.
2. Resolve the intended app and exact window with `app {action:"list"}` and
   `window {action:"list",app:...}`. Prefer the returned `window_id` over a fuzzy
   title, frontmost window or shared desktop coordinates.
3. Observe that window with `see` for pixels and Accessibility, or `inspect_ui` for
   text only. Use `fresh:true`; request `include_elements:true` when consuming the
   structured `_meta.ui_elements` table. Copy `_meta.snapshot_id` and element IDs
   exactly. A screenshot response includes inline image content.
4. Perform one intended mutation, bound to the fresh snapshot and its element ID
   or exact-window coordinate receipt. Background actions are preferred. The
   configured server allows foreground input, but set `foreground:true` only when
   taking focus or the shared pointer is intentional. Pin foreground input to the
   intended app/window rather than relying on whatever is currently focused.
5. Verify the expected effect with a **new** `see`/`inspect_ui` observation of the
   same window, `verify_state`, or a task-specific readback (for example, saved
   file contents). Continue only when the expected state is observed; report an
   unresolved outcome instead of claiming success.

## Unverified delivery is not a failed-to-run action

Peekaboo 4.9.2 can return `isError:true` even after a successful click, typing or
key chord. Read `_meta`: `mutation_dispatched:true`, `state:"dispatched_unverified"`,
`retry_safe:false` and `requires_fresh_observation:true` mean input was sent but
its effect was not confirmed by the provider.

**Observe before retry.** A fresh observation may already prove the requested
state; then the operation is complete. Blindly repeating a click, paste, chord or
text input can perform it twice. If the effect remains unknown, stop and explain
that uncertainty. For a pre-dispatch rejection with `mutation_dispatched:false`
and `retry_safe:true`, correct the target/parameters before trying again.

Use a fresh snapshot after mutations; cached or historical element IDs are not
new evidence. Prefer window-only captures to keep unrelated private UI out of
the model context.

Primary reference: [Peekaboo MCP](https://github.com/openclaw/Peekaboo/blob/v4.9.2/docs/MCP.md).
