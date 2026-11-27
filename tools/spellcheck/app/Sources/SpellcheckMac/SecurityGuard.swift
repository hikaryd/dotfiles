import AppKit
import ApplicationServices
import Carbon
import SpellcheckCore

@MainActor
final class SecurityGuard {
    private let ownPID = ProcessInfo.processInfo.processIdentifier
    private let axTimeoutSeconds: Float = 0.05

    func frontmostApp() -> NSRunningApplication? {
        NSWorkspace.shared.frontmostApplication
    }

    func frontmostBundleID() -> String? {
        frontmostApp()?.bundleIdentifier
    }

    func frontmostAppName() -> String {
        frontmostApp()?.localizedName ?? frontmostBundleID() ?? "Unknown"
    }

    func isSecureInputActive() -> Bool {
        if IsSecureEventInputEnabled() {
            return true
        }
        guard let focused = focusedAXElement() else { return false }
        if axStringAttribute(focused, kAXRoleAttribute as CFString) == "AXSecureTextField" {
            return true
        }
        if axStringAttribute(focused, kAXSubroleAttribute as CFString) == "AXSecureTextField" {
            return true
        }
        return false
    }

    func shouldMonitor(config: AppConfig) -> Bool {
        guard let app = frontmostApp() else { return false }
        if app.processIdentifier == ownPID { return false }
        if config.isExcluded(bundleID: app.bundleIdentifier) { return false }
        if isSecureInputActive() { return false }
        return true
    }

    /// Returns screen rect (in AppKit bottom-left coordinates) of the active text caret, if exposed by AX.
    func focusedCaretScreenRect() -> NSRect? {
        guard let focused = focusedAXElement() else { return nil }
        var rangeRef: CFTypeRef?
        let rangeErr = AXUIElementCopyAttributeValue(
            focused,
            kAXSelectedTextRangeAttribute as CFString,
            &rangeRef
        )
        guard rangeErr == .success, let rangeVal = rangeRef else { return nil }

        var boundsRef: CFTypeRef?
        let boundsErr = AXUIElementCopyParameterizedAttributeValue(
            focused,
            kAXBoundsForRangeParameterizedAttribute as CFString,
            rangeVal,
            &boundsRef
        )
        guard boundsErr == .success, let boundsVal = boundsRef,
              CFGetTypeID(boundsVal) == AXValueGetTypeID()
        else { return nil }

        var axRect = CGRect.zero
        guard AXValueGetValue(boundsVal as! AXValue, .cgRect, &axRect) else { return nil }
        guard axRect.width >= 0, axRect.height >= 0,
              axRect.origin.x.isFinite, axRect.origin.y.isFinite,
              !(axRect.origin.x == 0 && axRect.origin.y == 0 && axRect.width == 0 && axRect.height == 0)
        else { return nil }

        // AX coordinates use top-left of the primary screen (screens[0]); AppKit uses bottom-left.
        guard let primaryScreen = NSScreen.screens.first else { return nil }
        let primaryHeight = primaryScreen.frame.height
        let appKitY = primaryHeight - axRect.origin.y - axRect.height
        return NSRect(x: axRect.origin.x, y: appKitY, width: max(1, axRect.width), height: max(14, axRect.height))
    }

    private func focusedAXElement() -> AXUIElement? {
        let systemWide = AXUIElementCreateSystemWide()
        AXUIElementSetMessagingTimeout(systemWide, axTimeoutSeconds)
        var focusedRef: CFTypeRef?
        let err = AXUIElementCopyAttributeValue(
            systemWide,
            kAXFocusedUIElementAttribute as CFString,
            &focusedRef
        )
        guard err == .success, let ref = focusedRef,
              CFGetTypeID(ref) == AXUIElementGetTypeID()
        else { return nil }
        let elem = (ref as! AXUIElement)
        AXUIElementSetMessagingTimeout(elem, axTimeoutSeconds)
        return elem
    }

    private func axStringAttribute(_ element: AXUIElement, _ attr: CFString) -> String? {
        var ref: CFTypeRef?
        let err = AXUIElementCopyAttributeValue(element, attr, &ref)
        guard err == .success else { return nil }
        return ref as? String
    }
}
