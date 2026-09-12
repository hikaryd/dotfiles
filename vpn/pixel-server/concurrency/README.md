# Optional Pixel browser-use scheduler

This component is independent of VPN installation. It is a cooperative scheduler,
not a sandbox: arbitrary Python, raw CDP, other clients, and root can bypass it.
Chrome cookies/profile remain shared. Never describe jobs as isolated profiles.

## Contract

Use `browser_exec(session="pixel_read_1", code=...)` for an atomic read job.
Reuse two reserved worker names, `pixel_read_1` and `pixel_read_2`; concurrent
jobs must use different names. Structured reads use a direct CDP connection in
the CLI process, without creating harness daemons or extra idle tabs. Legacy
named sessions still create upstream daemons; avoid unlimited legacy names. The entire code must be:

```text
# pixel:read-v1
{"url":"https://example.org/","max_chars":20000}
```

Only HTTP(S) navigation plus fixed title/URL/body-text extraction is allowed;
caller JavaScript, input, focus, screenshots, and arbitrary CDP are not accepted
in this mode. Two read calls can run simultaneously. Each creates a background
tab and closes its own tab in `finally`. Output is JSON, capped to `max_chars`
characters of text (1–100000; default 20000). Readiness is not a guarantee that an
SPA finished loading. Pages themselves can execute scripts or change cookies.

Other Python remains legacy mode and acquires both slots exclusively. A named
legacy session attaches to its recorded own tab, which persists across calls.
Call the following **with the same session** when the task finishes:

```text
# pixel:finish-v1
```

Finish first verifies the recorded target is absent, then requests shutdown of
the current named harness daemon through its existing local IPC. Its upstream
dedicated idle blank is removed by the daemon finalizer; the response reports
shutdown requested, not proof the finalizer completed. Until explicit finish,
legacy daemons and their idle blank tabs may remain.

The default unnamed legacy session preserves current-tab compatibility, still
exclusive. There is no reliable automatic whole-conversation completion signal.
Existing legacy tabs are not adopted or automatically closed. The scheduler
only closes target IDs it created and recorded. It does not intercept arbitrary
legacy code that intentionally switches to other tabs.

Owner VNC retains the existing priority mechanism: it disconnects the Pixel CDP
tunnel. Calls may fail while owner mode is active; retry only after owner stop.
The bridge observes the owner lease by polling, not an atomic handoff: a short
job can finish during the transition before the tunnel drops. Do not claim
instant owner/agent mutual exclusion or retry an interrupted UI action blindly.
No browser fallback is introduced. A failed cleanup retains its record; the next
scheduled call reaps dead read workers using Linux PID + process start ticks.
No janitor runs while idle, and cleanup cannot succeed while CDP is unavailable.
The tiny create-target/record-write crash window is not transactionally covered.

## Optional installation (no gateway restart)

1. Run the tests and compile checks below. Copy `runtime.py` and `shim.py` into a
   private dedicated directory. Use absolute paths in the private JSON config.
2. In the installed Hermes Python environment, extract **the exact current**
   `tools.browser_use_cli._OWN_TAB_PREAMBLE` into a private text file. Do not copy
   a guessed version. It is stripped only on exact prefix match, before our
   strict pinning. Every named Pixel invocation must include this exact prefix;
   absent/changed prefixes fail closed, including direct CLI tests. If upstream
   changes the preamble, update this file first.
3. Write `~/.config/pixel-browser/scheduler.json` (directory 0700, file 0600):

```json
{
  "upstream_argv": ["/ABSOLUTE/INSTALLED/python", "/ABSOLUTE/ORIGINAL/browser-use"],
  "read_python": "/ABSOLUTE/INSTALLED/python",
  "runtime_file": "/ABSOLUTE/PRIVATE/runtime.py",
  "preamble_file": "/ABSOLUTE/PRIVATE/upstream-preamble.txt",
  "state_dir": "/ABSOLUTE/PRIVATE/state",
  "pixel_cdp_urls": ["http://127.0.0.1:19222", "ws://127.0.0.1:19222/devtools/browser"],
  "wait_seconds": 60,
  "preexisting_sessions": []
}
```

`PIXEL_SCHEDULER_CONFIG` overrides the config path. Pin the existing installed
Python and browser-use wrapper, not a package runner which could spawn a child
and break timeout ownership. `read_python` must already provide websockets
15.0.1; no package installation is performed. Never point upstream_argv back to
the shim. Direct reads accept a configured HTTP(S) CDP origin or an exact allowlisted
WS(S) browser endpoint. Explicit WS endpoints must use `/devtools/browser` or
`/devtools/browser/<safe-id>` without credentials, query, or fragment. HTTP discovery uses `/json/version` without redirects or environment
proxies. Its advertised WS path is bound to that same configured origin.

The direct transport follows [websockets 15.0.1 sync client API](https://websockets.readthedocs.io/en/15.0.1/reference/sync/client.html):
`proxy=None`, bounded open/close/receive timeouts and message size.

4. Before activation, capture verified existing daemon session names into the
   private `preexisting_sessions` list. Their legacy calls remain exclusive but
   keep their current tab when no managed manifest exists. Finish reports
   `preserved_unmanaged` and does not close or shut down those old sessions.
   Existing managed manifests take precedence. Old sessions are not automatically
   cleaned. Use new `pixel_ui_<task>` names for managed interactive tasks.
   Back up any existing `$HERMES_HOME/bin/browser-use`. Atomically install an
   executable copy of `shim.py` there. The loaded Hermes `_find_cli()` must be
   verified to rediscover this location on each call. No service restart needed.
   Non-Pixel legacy code and CLI option invocations delegate unchanged. Pixel
   directives on unconfigured endpoints fail closed rather than being passed
   to the upstream CLI as ordinary Python.
5. Verify real overlapping reads, exclusive legacy work, exact tab cleanup,
   owner-mode interruption/recovery, and killed-worker cleanup. Cleanup requires
   a validated target list proving absence, not just a close acknowledgement. Check gateway PID
   unchanged. Check both BU_CDP_URL and BU_CDP_WS routing against the installed
   harness; only configured exact endpoint strings activate scheduling. Set only
   one CDP environment variable for Pixel: conflicting URL/WS values fail closed.
6. Update the Hermes operational runbook: atomic read directive, two reusable worker sessions,
   explicit legacy finish, owner priority, shared cookie caveat, and RF services
   never using extra proxies. Existing active contexts need an explicit reminder.

Rollback: atomically restore the previous managed CLI (or remove only the shim
if none existed). Do not kill the gateway or close unrecorded personal tabs.

## Local verification

```sh
python3 -m unittest discover -s vpn/pixel-server/concurrency -p 'test_*.py'
python3 -m py_compile vpn/pixel-server/concurrency/runtime.py vpn/pixel-server/concurrency/shim.py
```

Tests use fake CDP and real cross-process POSIX locks. They do not prove actual
browser/owner tunnel behavior; deployment needs the live checks above.
