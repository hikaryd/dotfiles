import Foundation
import AppKit
import ApplicationServices
import Darwin

#if TEST
let directory = URL(fileURLWithPath: ProcessInfo.processInfo.environment["REX_TEST_DIR"]!)
#else
let directory = URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent("Library/Caches/dots-rex-last-session")
#endif
let historyFile = directory.appendingPathComponent("history.json")

func valid(_ id: String) -> Bool {
    guard id.hasPrefix("session:") else { return false }
    let value = String(id.dropFirst(8))
    return UUID(uuidString: value) != nil && value.count == 36
}

func lock(_ name: String, nonblocking: Bool = false) throws -> Int32 {
    let fd = open(directory.appendingPathComponent(name).path, O_CREAT | O_RDWR | O_CLOEXEC, 0o600)
    guard fd >= 0 else { throw NSError(domain: "lock open", code: Int(errno)) }
    guard flock(fd, LOCK_EX | (nonblocking ? LOCK_NB : 0)) == 0 else {
        let code = errno
        close(fd)
        throw NSError(domain: "lock busy", code: Int(code))
    }
    return fd
}

func readHistory() throws -> [String] {
    guard FileManager.default.fileExists(atPath: historyFile.path) else { return [] }
    let ids = try JSONDecoder().decode([String].self, from: Data(contentsOf: historyFile))
    guard ids.allSatisfy(valid), Set(ids).count == ids.count else { throw NSError(domain: "invalid history", code: 1) }
    return ids
}

func update(_ current: String, live: Set<String>? = nil, resetCorrupt: Bool = false, observed: Bool = false, requireReady: Bool = false) throws -> String? {
    let fd = try lock("history.lock")
    defer { close(fd) }
    let old: [String]
    do { old = try readHistory() } catch {
        guard resetCorrupt else { throw error }
        fputs("rex-last-session: corrupt history reset\n", stderr)
        let marker = directory.appendingPathComponent("ready")
        if FileManager.default.fileExists(atPath: marker.path) { try FileManager.default.removeItem(at: marker) }
        try? FileManager.default.removeItem(at: historyFile)
        return try writeHistory([current], target: nil)
    }
    if requireReady && !FileManager.default.fileExists(atPath: directory.appendingPathComponent("ready").path) {
        throw NSError(domain: "watcher has insufficient valid session history", code: 1)
    }
    if requireReady && old.first != current { throw NSError(domain: "selection still settling, retry", code: 1) }
    var ids = old.filter { $0 != current && (live == nil || live!.contains($0)) }
    let target = ids.first
    ids.insert(current, at: 0)
    if ids != old { _ = try writeHistory(ids, target: target) }
    let marker = directory.appendingPathComponent("ready")
    if observed && ids.count >= 2 && !FileManager.default.fileExists(atPath: marker.path) { try Data().write(to: marker, options: .atomic) }
    return target
}

func writeHistory(_ ids: [String], target: String?) throws -> String? {
    try JSONEncoder().encode(ids).write(to: historyFile, options: .atomic)
    return target
}

func matchTitle(_ title: String, sessions: [[String: Any]]) throws -> String {
    var matches: [String] = []
    for session in sessions {
        guard let id = session["session_id"] as? String, valid(id), let label = session["label"] as? String else { throw NSError(domain: "invalid session list", code: 1) }
        if title == label || title.hasSuffix(" - " + label) { matches.append(id) }
    }
    guard matches.count == 1 else { throw NSError(domain: "ambiguous or unknown session title", code: 1) }
    return matches[0]
}

#if !TEST
func invalidate(_ reason: String) throws {
    let fd = try lock("history.lock")
    defer { close(fd) }
    let marker = directory.appendingPathComponent("ready")
    if FileManager.default.fileExists(atPath: marker.path) { try FileManager.default.removeItem(at: marker) }
    _ = try writeHistory([], target: nil)
    fputs("rex-last-session: \(reason)\n", stderr)
}

func attribute(_ element: AXUIElement, _ name: String) -> CFTypeRef? {
    var value: CFTypeRef?
    guard AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success else { return nil }
    return value
}

func resolve(_ title: String) throws -> String {
    let output = directory.appendingPathComponent("sessions-output")
    let fd = open(output.path, O_CREAT | O_TRUNC | O_WRONLY | O_CLOEXEC, 0o600)
    guard fd >= 0 else { throw NSError(domain: "session output open failed", code: Int(errno)) }
    let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
    defer { try? handle.close(); try? FileManager.default.removeItem(at: output) }
    let process = Process()
    process.executableURL = URL(fileURLWithPath: NSHomeDirectory()).appendingPathComponent(".local/bin/rex")
    process.arguments = ["--autostart=false", "--timeout", "2s", "ls", "--json"]
    process.standardOutput = handle
    process.standardError = FileHandle.nullDevice
    let done = DispatchSemaphore(value: 0)
    process.terminationHandler = { _ in done.signal() }
    try process.run()
    if done.wait(timeout: .now() + 3) == .timedOut {
        if process.isRunning { process.terminate() }
        throw NSError(domain: "session lookup timed out", code: 1)
    }
    guard process.terminationStatus == 0 else { throw NSError(domain: "session lookup failed", code: 1) }
    let data = try Data(contentsOf: output)
    guard data.count <= 1_000_000,
          let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
          let sessions = json["sessions"] as? [[String: Any]] else { throw NSError(domain: "invalid session list", code: 1) }
    return try matchTitle(title, sessions: sessions)
}

var appElement: AXUIElement?
var observer: AXObserver?
var attachedPID: pid_t = 0
var warned = false

func failObservation(_ reason: String) {
    do { try invalidate(reason) } catch {
        fputs("rex-last-session: cannot invalidate history: \(error)\n", stderr)
        exit(1)
    }
}

func observeWindow() {
    guard let app = appElement, let observer else { return }
    if let windows = attribute(app, kAXWindowsAttribute) as? [AXUIElement] {
        for window in windows {
            let result = AXObserverAddNotification(observer, window, kAXTitleChangedNotification as CFString, nil)
            if result != .success && result != .notificationAlreadyRegistered { failObservation("cannot observe window title"); return }
        }
    }
    let gate: Int32
    do { gate = try lock("resolve.lock") } catch { failObservation("cannot lock session observation"); return }
    defer { close(gate) }
    guard let focused = attribute(app, kAXFocusedWindowAttribute), CFGetTypeID(focused) == AXUIElementGetTypeID() else { failObservation("cannot determine focused window"); return }
    let window = focused as! AXUIElement
    guard let identifier = attribute(window, kAXIdentifierAttribute) as? String, identifier.hasPrefix("session-AppWindow-") else { return }
    guard let title = attribute(window, kAXTitleAttribute) as? String else {
        failObservation("cannot read session window title")
        return
    }
    // shortcut: serial lookups can miss rapid title changes; replace when Rex exposes session events.
    do { _ = try update(resolve(title), resetCorrupt: true, observed: true) }
    catch { failObservation("session observation failed: \(error)") }
}

func axCallback(_ observer: AXObserver, _ element: AXUIElement, _ notification: CFString, _ context: UnsafeMutableRawPointer?) {
    if notification as String == kAXTitleChangedNotification, let app = appElement,
       let focused = attribute(app, kAXFocusedWindowAttribute), !CFEqual(focused, element) { return }
    observeWindow()
}

func refresh() {
    guard AXIsProcessTrusted() else {
        if !warned {
            failObservation("Accessibility permission required; run dots-rex-last-session request-access")
            attachedPID = 0
            observer = nil
            appElement = nil
            warned = true
        }
        return
    }
    warned = false
    guard let app = NSRunningApplication.runningApplications(withBundleIdentifier: "com.superlogical.rex").first else {
        if attachedPID != 0 { failObservation("Rex is not running"); attachedPID = 0; observer = nil; appElement = nil }
        return
    }
    guard app.processIdentifier != attachedPID else { return }
    failObservation("attaching to Rex")
    var newObserver: AXObserver?
    guard AXObserverCreate(app.processIdentifier, axCallback, &newObserver) == .success, let newObserver else { return }
    let element = AXUIElementCreateApplication(app.processIdentifier)
    for name in [kAXFocusedWindowChangedNotification, kAXMainWindowChangedNotification, kAXWindowCreatedNotification] {
        guard AXObserverAddNotification(newObserver, element, name as CFString, nil) == .success else { return }
    }
    appElement = element
    observer = newObserver
    attachedPID = app.processIdentifier
    CFRunLoopAddSource(CFRunLoopGetCurrent(), AXObserverGetRunLoopSource(newObserver), .defaultMode)
    observeWindow()
}

func run() throws {
    umask(0o077)
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
    guard chmod(directory.path, 0o700) == 0 else { throw NSError(domain: "cache permissions", code: Int(errno)) }
    let args = Array(CommandLine.arguments.dropFirst())
    guard let mode = args.first else { throw NSError(domain: "expected watch or target", code: 1) }
    if mode == "watch", args.count == 1 {
        let gate = try lock("resolve.lock")
        let fd: Int32
        do {
            fd = try lock("watch.lock", nonblocking: true)
            try invalidate("watcher starting")
        } catch { close(gate); throw error }
        close(gate)
        defer { close(fd) }
        refresh()
        Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { _ in refresh() }
        RunLoop.current.run()
    } else if mode == "request-access", args.count == 1 {
        let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
        print(AXIsProcessTrustedWithOptions(options) ? "Accessibility access granted" : "Grant Accessibility access to dots-rex-last-session in System Settings")
    } else if mode == "target", args.count >= 2 {
        let current = args[1], ids = Array(args.dropFirst(2))
        guard valid(current), ids.allSatisfy(valid), ids.contains(current) else { throw NSError(domain: "invalid current or live sessions", code: 1) }
        let gate = try lock("resolve.lock", nonblocking: true)
        defer { close(gate) }
        do {
            let fd = try lock("watch.lock", nonblocking: true)
            close(fd)
            throw NSError(domain: "watcher is not running", code: 1)
        } catch let error as NSError {
            guard error.domain == "lock busy" && (error.code == EWOULDBLOCK || error.code == EAGAIN) else { throw error }
        }
        guard let target = try update(current, live: Set(ids), requireReady: true) else { throw NSError(domain: "no previous session", code: 1) }
        print(target)
    } else { throw NSError(domain: "invalid arguments", code: 1) }
}

do { try run() } catch {
    fputs("rex-last-session: \(error)\n", stderr)
    exit(1)
}
#else
func test() throws {
    umask(0o077)
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    let a = "session:AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA"
    let b = "session:BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB"
    let c = "session:CCCCCCCC-CCCC-CCCC-CCCC-CCCCCCCCCCCC"
    func check(_ value: Bool) { assert(value) }
    let samples: [[String: Any]] = [["session_id": a, "label": "work"], ["session_id": b, "label": "home"]]
    try check(matchTitle("tab - work", sessions: samples) == a)
    try check(matchTitle("home", sessions: samples) == b)
    for (title, values) in [("unknown", samples), ("tab - work", [["session_id": a, "label": "work"], ["session_id": b, "label": "work"]]), ("tab - x - work", [["session_id": a, "label": "work"], ["session_id": b, "label": "x - work"]])] {
        do { _ = try matchTitle(title, sessions: values); fatalError("ambiguous title accepted") } catch {}
    }
    assert(valid(a) && !valid("session:bad"))
    do { _ = try update(a, requireReady: true); fatalError("unready history accepted") } catch {}
    try check(update(a, observed: true) == nil)
    do { _ = try update(b, requireReady: true); fatalError("single observation accepted") } catch {}
    try check(update(b, observed: true) == a)
    do { _ = try update(a, live: [a, b], requireReady: true); fatalError("stale observation accepted") } catch {}
    try check(readHistory() == [b, a])
    try check(update(b, live: [a, b], requireReady: true) == a)
    try check(update(b) == a)
    try check(update(c) == b)
    try check(update(c) == b)
    try check(update(b, live: [a, b, c]) == c)
    try check(update(c, live: [a, b, c]) == b)
    try check(update(b, live: [a, b]) == a)
    try check(update(c, live: [c]) == nil)
    try Data("bad".utf8).write(to: historyFile)
    do { _ = try update(a); fatalError("corrupt cache accepted") } catch {}
    try check(update(a, resetCorrupt: true) == nil)
    let gate = try lock("resolve.lock")
    do { let fd = try lock("resolve.lock", nonblocking: true); close(fd); fatalError("busy observation accepted") } catch {}
    close(gate)
    let owner = try lock("watch.lock", nonblocking: true)
    do { let fd = try lock("watch.lock", nonblocking: true); close(fd); fatalError("duplicate watcher accepted") } catch {}
    close(owner)
    let free = try lock("watch.lock", nonblocking: true)
    close(free)
    DispatchQueue.concurrentPerform(iterations: 12) { i in
        let id = String(format: "session:%08X-0000-0000-0000-000000000000", i + 1)
        do { _ = try update(id) } catch { fatalError("concurrent update failed: \(error)") }
    }
    try check(readHistory().count == 13)
    print("PASS Rex last-session MRU and locking")
}
do { try test() } catch { fputs("test failed: \(error)\n", stderr); exit(1) }
#endif
