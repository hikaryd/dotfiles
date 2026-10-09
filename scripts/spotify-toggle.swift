import AppKit
import ApplicationServices
import CoreGraphics

guard let app = NSRunningApplication.runningApplications(withBundleIdentifier: "com.spotify.client").first else {
    print("launch")
    exit(0)
}

if !app.isFinishedLaunching {
    print("launch")
    exit(0)
}

func hasVisibleWindow() -> Bool {
    let windows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
    return windows.contains { ($0[kCGWindowOwnerPID as String] as? Int) == Int(app.processIdentifier) }
}

func waitForWindow(visible: Bool) -> Bool {
    for _ in 0..<80 {
        if hasVisibleWindow() == visible { return true }
        usleep(25_000)
    }
    return false
}

if hasVisibleWindow() {
    app.hide()
    guard waitForWindow(visible: false) else {
        fputs("spotify-toggle: Spotify did not hide\n", stderr)
        exit(1)
    }
    print("hidden")
} else {
    let axApp = AXUIElementCreateApplication(app.processIdentifier)
    var rawWindows: CFTypeRef?
    if AXUIElementCopyAttributeValue(axApp, kAXWindowsAttribute as CFString, &rawWindows) == .success,
       let axWindows = rawWindows as? [AXUIElement] {
        for window in axWindows {
            AXUIElementSetAttributeValue(window, kAXMinimizedAttribute as CFString, kCFBooleanFalse)
        }
    }
    app.unhide()
    app.activate(options: [.activateAllWindows])
    guard waitForWindow(visible: true) else {
        fputs("spotify-toggle: could not show Spotify\n", stderr)
        exit(1)
    }
    print("shown")
}
