#!/usr/bin/env bash
# Explicit desktop regression: invokes the real hotkey entry point. Never in CI.
# Disable the notch widget and start with the main window shown.
# The successful test restores it; hide must not minimize the window in Dock.
set -euo pipefail
[[ $# -eq 1 && ( $1 == --allow-ui || $1 == --check-hidden ) ]] || {
  echo 'Usage: tests/test-spotifast-toggle-live.sh --allow-ui|--check-hidden' >&2
  exit 2
}
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-spotifast-live.XXXXXX")
trap 'rm -rf "$work"' EXIT
cat > "$work/main.swift" <<'SWIFT'
import AppKit
import ApplicationServices
import CoreGraphics

guard let app = NSRunningApplication.runningApplications(withBundleIdentifier: "rocks.spotifast.Spotifast").first else {
    fputs("FAIL: Spotifast is not running\n", stderr)
    exit(2)
}
let windows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
let visibleMainWindows = windows.filter {
    ($0[kCGWindowOwnerPID as String] as? Int) == Int(app.processIdentifier) &&
    ($0[kCGWindowLayer as String] as? Int) == 0
}.count
var rawWindows: CFTypeRef?
let axApp = AXUIElementCreateApplication(app.processIdentifier)
guard AXUIElementCopyAttributeValue(axApp, kAXWindowsAttribute as CFString, &rawWindows) == .success,
      let axWindows = rawWindows as? [AXUIElement] else {
    fputs("FAIL: cannot inspect Spotifast accessibility windows\n", stderr)
    exit(2)
}
let minimized = axWindows.compactMap { window -> Bool? in
    var settable: DarwinBoolean = false
    guard AXUIElementIsAttributeSettable(window, kAXMinimizedAttribute as CFString, &settable) == .success,
          settable.boolValue else { return nil }
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(window, kAXMinimizedAttribute as CFString, &value) == .success else { return nil }
    return (value as? NSNumber)?.boolValue
}
guard !minimized.isEmpty else { fputs("FAIL: no standard Spotifast windows found\n", stderr); exit(2) }
let expected = CommandLine.arguments[1]
let passed = expected == "hidden"
    ? visibleMainWindows == 0 && app.isHidden && minimized.allSatisfy { !$0 }
    : visibleMainWindows > 0 && minimized.contains(false)
print("\(passed ? "PASS" : "FAIL"): expected=\(expected) appHidden=\(app.isHidden) onScreenMainWindows=\(visibleMainWindows) minimized=\(minimized)")
exit(passed ? 0 : 1)
SWIFT
/usr/bin/swiftc -O "$work/main.swift" -o "$work/state"
if [[ $1 == --check-hidden ]]; then
  "$work/state" hidden
  exit 0
fi
"$work/state" shown
"$HOME/.local/bin/spotifast-toggle"
sleep 1
"$work/state" hidden
"$HOME/.local/bin/spotifast-toggle"
sleep 1
"$work/state" shown
printf 'PASS real Spotifast native hide/show round trip without Dock minimization\n'
