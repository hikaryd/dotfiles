import AppKit
import Carbon
import CoreGraphics
import os
import SpellcheckCore

/// Shared lock-protected state read inside the dedicated-thread CGEventTap callback without blocking on MainActor.
private final class TapSharedState: Sendable {
    struct State: Sendable {
        var keystrokeEpoch: UInt64 = 0
        var popoverVisible: Bool = false
        var popoverAppPID: pid_t = 0
        var undoWindowActive: Bool = false
        var undoAppPID: pid_t = 0
        var frontmostPID: pid_t = 0

        var isInjecting: Bool = false
        var injectionGeneration: UInt64 = 0
        var injectionTargetPID: pid_t = 0
        var abortInjection: Bool = false
        var deferredEvents: [SendableCGEvent] = []
    }

    private let lock = OSAllocatedUnfairLock(initialState: State())

    func read() -> State {
        lock.withLock { $0 }
    }

    func bumpEpoch() -> UInt64 {
        lock.withLock { state in
            state.keystrokeEpoch &+= 1
            return state.keystrokeEpoch
        }
    }

    func setFrontmostPID(_ pid: pid_t) {
        lock.withLock { state in
            state.frontmostPID = pid
            if state.isInjecting && state.injectionTargetPID != 0 && state.injectionTargetPID != pid {
                state.abortInjection = true
                state.deferredEvents.removeAll()
            }
        }
    }

    func setPopoverState(visible: Bool, appPID: pid_t = 0) {
        lock.withLock { state in
            state.popoverVisible = visible
            state.popoverAppPID = visible ? appPID : 0
        }
    }

    func setUndoState(active: Bool, appPID: pid_t = 0) {
        lock.withLock { state in
            state.undoWindowActive = active
            state.undoAppPID = active ? appPID : 0
        }
    }

    /// Atomically arms injection if no newer user keystroke has arrived since `expectedEpoch` and no injection is active.
    func beginInjectionIfEpochMatches(expectedEpoch: UInt64, targetPID: pid_t) -> UInt64? {
        lock.withLock { state in
            guard !state.isInjecting,
                  state.keystrokeEpoch == expectedEpoch,
                  state.frontmostPID == targetPID
            else { return nil }
            state.injectionGeneration &+= 1
            state.isInjecting = true
            state.injectionTargetPID = targetPID
            state.abortInjection = false
            state.deferredEvents.removeAll()
            return state.injectionGeneration
        }
    }

    /// Atomically arms injection unconditionally (if idle) when triggered by a click or consumed shortcut.
    func beginInjection(targetPID: pid_t) -> UInt64? {
        lock.withLock { state in
            guard !state.isInjecting else { return nil }
            state.injectionGeneration &+= 1
            state.isInjecting = true
            state.injectionTargetPID = targetPID
            state.abortInjection = false
            state.deferredEvents.removeAll()
            return state.injectionGeneration
        }
    }

    func canContinueInjection(generation: UInt64, expectedPID: pid_t) -> Bool {
        lock.withLock { state in
            state.isInjecting
                && state.injectionGeneration == generation
                && !state.abortInjection
                && state.frontmostPID == expectedPID
        }
    }

    func abortCurrentInjection() {
        lock.withLock { state in
            if state.isInjecting {
                state.abortInjection = true
            }
            state.deferredEvents.removeAll()
        }
    }

    func deferEvent(_ wrapped: SendableCGEvent) -> Bool {
        lock.withLock { state in
            guard state.isInjecting, !state.abortInjection, state.deferredEvents.count < 16 else {
                return false
            }
            state.deferredEvents.append(wrapped)
            return true
        }
    }

    /// Called by TextInjector once Backspaces and replacement text are posted.
    /// Keeps `isInjecting = true` while returning any buffered keystrokes to replay;
    /// clears `isInjecting = false` atomically only when `deferredEvents` is empty.
    func popDeferredOrFinish(generation: UInt64) -> PopDeferredResult {
        lock.withLock { state in
            guard state.injectionGeneration == generation, state.isInjecting else {
                return .abortedOrSuperseded
            }
            if state.abortInjection {
                state.isInjecting = false
                state.injectionTargetPID = 0
                state.abortInjection = false
                state.deferredEvents.removeAll()
                return .abortedOrSuperseded
            }
            if !state.deferredEvents.isEmpty {
                let batch = state.deferredEvents
                state.deferredEvents.removeAll()
                return .batch(batch)
            }
            state.isInjecting = false
            state.injectionTargetPID = 0
            return .finishedCleanly
        }
    }

    /// Aborts and clears `generation` if it is still the active injection. Returns true iff this generation was current.
    @discardableResult
    func finishAborted(generation: UInt64) -> Bool {
        lock.withLock { state in
            guard state.injectionGeneration == generation else {
                return false
            }
            state.isInjecting = false
            state.injectionTargetPID = 0
            state.abortInjection = false
            state.deferredEvents.removeAll()
            return true
        }
    }
}

private struct SendableBox<T>: @unchecked Sendable {
    let value: T
}

/// Manages a CGEventTap on a dedicated high-priority thread so MainActor UI or AX IPC can never stall system input.
/// Thread-safe: immutable reference owned by TypingMonitor; `stop()` handles pre-CFRunLoopRun races and waits for tap teardown.
private final class DedicatedTapRunner: @unchecked Sendable {
    private struct TapHandles: @unchecked Sendable {
        var tap: CFMachPort?
        var runLoop: CFRunLoop?
        var running: Bool = false
        var stopRequested: Bool = false
        var exitSem: DispatchSemaphore?
    }

    private let stateLock = OSAllocatedUnfairLock<TapHandles>(initialState: TapHandles())

    var isRunning: Bool {
        stateLock.withLock { $0.running }
    }

    func currentTapPort() -> CFMachPort? {
        stateLock.withLock { $0.tap.map(SendableBox.init) }?.value
    }

    func start(callback: @escaping CGEventTapCallBack, userInfo: UnsafeMutableRawPointer) -> Bool {
        if isRunning { return true }

        let startSem = DispatchSemaphore(value: 0)
        let exitSem = DispatchSemaphore(value: 0)
        let startedBox = OSAllocatedUnfairLock(initialState: false)
        let userInfoBox = SendableBox(value: userInfo)

        stateLock.withLock {
            $0.stopRequested = false
            $0.exitSem = exitSem
        }

        let thread = Thread { [weak self] in
            defer { exitSem.signal() }
            guard let self else {
                startSem.signal()
                return
            }

            let mask = CGEventMask(1 << CGEventType.keyDown.rawValue)
            guard let tap = CGEvent.tapCreate(
                tap: .cgSessionEventTap,
                place: .headInsertEventTap,
                options: .defaultTap,
                eventsOfInterest: mask,
                callback: callback,
                userInfo: userInfoBox.value
            ) else {
                self.stateLock.withLock {
                    $0.exitSem = nil
                }
                startSem.signal()
                return
            }

            let source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0)
            let rl = CFRunLoopGetCurrent()!
            CFRunLoopAddSource(rl, source, .commonModes)
            CGEvent.tapEnable(tap: tap, enable: true)

            let tapBox = SendableBox(value: tap)
            let rlBox = SendableBox(value: rl)
            let alreadyStopped: Bool = self.stateLock.withLock {
                $0.tap = tapBox.value
                $0.runLoop = rlBox.value
                $0.running = !$0.stopRequested
                return $0.stopRequested
            }
            startedBox.withLock { $0 = !alreadyStopped }
            startSem.signal()

            if !alreadyStopped {
                // Re-check under lock via a queued block so stop() called before CFRunLoopRun() always breaks out.
                CFRunLoopPerformBlock(rl, CFRunLoopMode.commonModes.rawValue) { [weak self] in
                    guard let self else { return }
                    let shouldStop = self.stateLock.withLock { $0.stopRequested }
                    if shouldStop {
                        CFRunLoopStop(rl)
                    }
                }
                CFRunLoopWakeUp(rl)
                CFRunLoopRun()
            }

            CGEvent.tapEnable(tap: tap, enable: false)
            CFRunLoopRemoveSource(rl, source, .commonModes)
            CFMachPortInvalidate(tap)
            self.stateLock.withLock {
                $0.tap = nil
                $0.runLoop = nil
                $0.running = false
                $0.exitSem = nil
            }
        }
        thread.name = "com.spellcheckmac.eventtap"
        thread.qualityOfService = .userInteractive
        thread.start()

        startSem.wait()
        return startedBox.withLock { $0 }
    }

    func stop() {
        let (rlBox, exitSem): (SendableBox<CFRunLoop>?, DispatchSemaphore?) = stateLock.withLock { state in
            state.stopRequested = true
            return (state.runLoop.map(SendableBox.init), state.exitSem)
        }
        if let rl = rlBox?.value {
            CFRunLoopPerformBlock(rl, CFRunLoopMode.commonModes.rawValue) {
                CFRunLoopStop(rl)
            }
            CFRunLoopWakeUp(rl)
            CFRunLoopStop(rl)
        }
        // Wait for CFMachPortInvalidate to complete so unretained callback refcon cannot dangle.
        if let exitSem {
            _ = exitSem.wait(timeout: .now() + .milliseconds(500))
        }
    }
}

private struct CapturedKeyEvent: Sendable {
    let epoch: UInt64
    let keyCode: CGKeyCode
    let flagsRaw: UInt64
    let unicode: String
}

@MainActor
final class TypingMonitor {
    private let securityGuard: SecurityGuard
    private let wordCorrector: WordCorrector
    private let typingBuffer = TypingBuffer()
    private let injector = TextInjector()
    private let sentenceClient: SentenceClient
    private let popover: SentencePopoverController
    private var configProvider: () -> AppConfig

    private let sharedState = TapSharedState()
    /// Immutable thread-safe runner so the background CGEventTap callback never touches mutable MainActor state.
    private let tapRunner = DedicatedTapRunner()
    private var mouseMonitor: Any?
    private var workspaceObserver: NSObjectProtocol?
    private var debounceTask: Task<Void, Never>?
    private var undoExpiryTask: Task<Void, Never>?

    private var activeSentencePending: PendingSentence?
    private var activeSentenceAppPID: pid_t?
    private var lastCorrectionAppPID: pid_t?
    private var lastCheckedSentence: String?

    var isTapRunning: Bool { tapRunner.isRunning }

    init(
        securityGuard: SecurityGuard,
        wordCorrector: WordCorrector,
        sentenceClient: SentenceClient,
        popover: SentencePopoverController,
        configProvider: @escaping () -> AppConfig
    ) {
        self.securityGuard = securityGuard
        self.wordCorrector = wordCorrector
        self.sentenceClient = sentenceClient
        self.popover = popover
        self.configProvider = configProvider

        if let pid = securityGuard.frontmostApp()?.processIdentifier {
            sharedState.setFrontmostPID(pid)
        }

        self.popover.onApply = { [weak self] suggestion, preArmedGen in
            self?.applySentenceSuggestion(suggestion, preArmedGeneration: preArmedGen)
        }
        self.popover.onApplyAborted = { [weak self] preArmedGen in
            if let gen = preArmedGen {
                self?.sharedState.finishAborted(generation: gen)
            }
        }
        self.popover.onDismiss = { [weak self] in
            self?.sharedState.setPopoverState(visible: false)
        }
    }

    @discardableResult
    func start() -> Bool {
        if isTapRunning { return true }

        if let pid = securityGuard.frontmostApp()?.processIdentifier {
            sharedState.setFrontmostPID(pid)
        }

        let refcon = Unmanaged.passUnretained(self).toOpaque()
        guard tapRunner.start(callback: Self.eventTapCallback, userInfo: refcon) else {
            return false
        }

        mouseMonitor = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.handleGlobalMouseClick()
            }
        }

        workspaceObserver = NSWorkspace.shared.notificationCenter.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self else { return }
                let pid = self.securityGuard.frontmostApp()?.processIdentifier ?? 0
                self.sharedState.setFrontmostPID(pid)
                self.resetState(dismissPopover: true)
            }
        }

        return true
    }

    func stop() {
        debounceTask?.cancel()
        undoExpiryTask?.cancel()
        sharedState.abortCurrentInjection()
        tapRunner.stop()
        if let monitor = mouseMonitor {
            NSEvent.removeMonitor(monitor)
            mouseMonitor = nil
        }
        if let obs = workspaceObserver {
            NSWorkspace.shared.notificationCenter.removeObserver(obs)
            workspaceObserver = nil
        }
        resetState(dismissPopover: true)
    }

    // MARK: - CGEventTap callback (runs on dedicated high-priority thread)

    private static let navigationKeyCodes: Set<CGKeyCode> = [
        36, 48, 53, 76, 115, 116, 117, 119, 121, 123, 124, 125, 126
    ]

    private static let eventTapCallback: CGEventTapCallBack = { _, type, event, refcon in
        guard let refcon else { return Unmanaged.passUnretained(event) }
        let monitor = Unmanaged<TypingMonitor>.fromOpaque(refcon).takeUnretainedValue()

        if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
            if let tap = monitor.tapRunner.currentTapPort() {
                CGEvent.tapEnable(tap: tap, enable: true)
            }
            return Unmanaged.passUnretained(event)
        }

        guard type == .keyDown else {
            return Unmanaged.passUnretained(event)
        }

        let userData = event.getIntegerValueField(.eventSourceUserData)

        // Synthetic Backspace / replacement events pass straight through untouched.
        if userData == TextInjector.selfEventTag {
            return Unmanaged.passUnretained(event)
        }

        let keyCode = CGKeyCode(event.getIntegerValueField(.keyboardEventKeycode))
        let flags = event.flags
        let significantMods: CGEventFlags = [.maskCommand, .maskControl, .maskAlternate]
        let activeMods = flags.intersection(significantMods)

        // Replayed deferred keystrokes bypass deferral and proceed straight to editor + TypingBuffer in FIFO order.
        let isReplayed = (userData == TextInjector.replayEventTag)

        let snap = monitor.sharedState.read()

        // While TextInjector is mid-replacement (or draining deferred events), buffer live normal keystrokes
        // so they replay in strict FIFO order, and abort immediately on Cmd/Ctrl/Esc/nav/Backspace.
        if !isReplayed && snap.isInjecting {
            if !activeMods.isEmpty || navigationKeyCodes.contains(keyCode) || keyCode == 51 {
                monitor.sharedState.abortCurrentInjection()
                _ = monitor.sharedState.bumpEpoch()
                return Unmanaged.passUnretained(event)
            }
            if let copied = event.copy(), monitor.sharedState.deferEvent(SendableCGEvent(event: copied)) {
                return nil
            }
            monitor.sharedState.abortCurrentInjection()
            _ = monitor.sharedState.bumpEpoch()
            return Unmanaged.passUnretained(event)
        }

        // Option+Return applies the currently visible sentence popover (pre-arms isInjecting atomically).
        if !isReplayed
            && snap.popoverVisible
            && snap.frontmostPID == snap.popoverAppPID
            && keyCode == 36
            && activeMods == .maskAlternate
            && !IsSecureEventInputEnabled()
        {
            if let gen = monitor.sharedState.beginInjection(targetPID: snap.frontmostPID) {
                monitor.sharedState.setPopoverState(visible: false)
                Task { @MainActor [weak monitor] in
                    monitor?.popover.triggerApplyCurrent(preArmedGeneration: gen)
                }
                return nil
            }
        }

        // Escape dismisses visible popover.
        if !isReplayed && snap.popoverVisible && keyCode == 53 && activeMods.isEmpty {
            monitor.sharedState.setPopoverState(visible: false)
            Task { @MainActor [weak monitor] in
                monitor?.popover.hide()
            }
            return nil
        }

        // Immediate unmodified Backspace right after a word autocorrect cleanly reverts it (pre-arms isInjecting atomically).
        if !isReplayed
            && snap.undoWindowActive
            && snap.frontmostPID == snap.undoAppPID
            && keyCode == 51
            && activeMods.isEmpty
            && !IsSecureEventInputEnabled()
        {
            if let gen = monitor.sharedState.beginInjection(targetPID: snap.frontmostPID) {
                monitor.sharedState.setUndoState(active: false)
                Task { @MainActor [weak monitor] in
                    monitor?.performUndoAutoCorrection(preArmedGeneration: gen)
                }
                return nil
            }
        }

        let epoch = monitor.sharedState.bumpEpoch()
        var chars = [UniChar](repeating: 0, count: 8)
        var length = 0
        event.keyboardGetUnicodeString(maxStringLength: chars.count, actualStringLength: &length, unicodeString: &chars)
        let unicode = length > 0 ? String(utf16CodeUnits: chars, count: length) : ""

        let captured = CapturedKeyEvent(epoch: epoch, keyCode: keyCode, flagsRaw: flags.rawValue, unicode: unicode)
        Task { @MainActor [weak monitor] in
            monitor?.processKeyEvent(captured)
        }
        return Unmanaged.passUnretained(event)
    }

    // MARK: - MainActor key processing

    private func processKeyEvent(_ ev: CapturedKeyEvent) {
        let cfg = configProvider()
        let frontPID = securityGuard.frontmostApp()?.processIdentifier ?? 0
        sharedState.setFrontmostPID(frontPID)

        guard securityGuard.shouldMonitor(config: cfg) else {
            resetState(dismissPopover: true)
            return
        }

        let flags = CGEventFlags(rawValue: ev.flagsRaw)

        // Command or Control shortcuts invalidate keystroke buffer (Cmd+V, Cmd+Z, Cmd+A, Ctrl+W, etc.).
        if flags.contains(.maskCommand) || flags.contains(.maskControl) {
            resetState(dismissPopover: true)
            return
        }

        // Navigation / Enter / Tab / Esc keys reset state.
        if Self.navigationKeyCodes.contains(ev.keyCode) {
            resetState(dismissPopover: true)
            return
        }

        // Any typing invalidates the previous auto-correction undo window (except unmodified Backspace handled above).
        clearUndoWindow()

        // Backspace: Option+Backspace (word delete) cannot be tracked accurately -> reset buffer safely.
        if ev.keyCode == 51 {
            if flags.contains(.maskAlternate) {
                resetState(dismissPopover: true)
                return
            }
            debounceTask?.cancel()
            sentenceClient.cancelPending()
            popover.hide()
            sharedState.setPopoverState(visible: false)
            typingBuffer.handleBackspace()
            lastCheckedSentence = nil
            if cfg.sentenceCheckEnabled, typingBuffer.currentPendingSentence() != nil {
                scheduleSentenceCheck(delaySeconds: cfg.debounceSeconds)
            }
            return
        }

        guard !ev.unicode.isEmpty else { return }

        let outcome = typingBuffer.handleTypedString(ev.unicode)

        if outcome.isPunctuationContinuation {
            // Keep pending 280 ms sentence check running if user pressed space right after `.`,
            // and keep already-visible popover's trailing whitespace synchronized so Apply works in place.
            if popover.isVisible, let updated = typingBuffer.currentPendingSentence(),
               let active = activeSentencePending, updated.sentence == active.sentence
            {
                activeSentencePending = updated
                popover.updateTrailingWhitespace(updated.trailingWhitespace)
            }
            if outcome.shouldScheduleQuickSentenceCheck {
                // Consecutive `.`/`!` (`...`, `?!`) -> cancel prior request and wait 280 ms for cluster to finish.
                sentenceClient.cancelPending()
                debounceTask?.cancel()
            }
        } else {
            if popover.isVisible {
                popover.hide()
                sharedState.setPopoverState(visible: false)
            }
            sentenceClient.cancelPending()
            debounceTask?.cancel()
        }

        // 1. Word-level instant auto-correction
        if cfg.wordAutocorrectEnabled,
           let word = outcome.completedWord,
           let delim = outcome.delimiter,
           let fixed = wordCorrector.suggestAutoCorrection(for: word)
        {
            // Atomically verify no newer keystroke arrived during AX/NSSpellChecker before arming injection.
            if let gen = sharedState.beginInjectionIfEpochMatches(expectedEpoch: ev.epoch, targetPID: frontPID) {
                let deleteCount = word.count + delim.count
                let insertText = fixed + delim

                typingBuffer.applyWordReplacement(original: word, replacement: fixed, delimiter: delim)
                wordCorrector.recordCorrection(original: word, replacement: fixed, delimiter: delim)
                armUndoWindow(seconds: 5.0, appPID: frontPID)

                runInjection(
                    generation: gen,
                    targetPID: frontPID,
                    deleteCount: deleteCount,
                    insertText: insertText,
                    settleMicroseconds: 5_000
                )
            }
        }

        // 2. Sentence-level check via local SAGE model
        guard cfg.sentenceCheckEnabled else { return }

        if outcome.shouldScheduleQuickSentenceCheck {
            // 280 ms settle delay handles `...` and `?!` cleanly in a single request.
            scheduleSentenceCheck(delaySeconds: 0.28)
        } else if outcome.shouldScheduleIdleDebounce {
            scheduleSentenceCheck(delaySeconds: cfg.debounceSeconds)
        }
    }

    private func performUndoAutoCorrection(preArmedGeneration gen: UInt64) {
        undoExpiryTask?.cancel()
        sharedState.setUndoState(active: false)
        sharedState.setPopoverState(visible: false)
        popover.hide()

        let cfg = configProvider()
        guard let expectedPID = lastCorrectionAppPID,
              securityGuard.frontmostApp()?.processIdentifier == expectedPID,
              securityGuard.shouldMonitor(config: cfg),
              let last = wordCorrector.consumeUndoIfRecent()
        else {
            sharedState.finishAborted(generation: gen)
            wordCorrector.clearLastCorrection()
            lastCorrectionAppPID = nil
            return
        }

        lastCorrectionAppPID = nil
        let deleteCount = last.replacement.count + last.delimiter.count
        let restoreText = last.original + last.delimiter
        typingBuffer.applyUndoWordReplacement(
            replacement: last.replacement,
            original: last.original,
            delimiter: last.delimiter
        )

        runInjection(
            generation: gen,
            targetPID: expectedPID,
            deleteCount: deleteCount,
            insertText: restoreText,
            settleMicroseconds: 2_000
        ) { [weak self] completed in
            guard completed else { return }
            Task { @MainActor [weak self] in
                guard let self else { return }
                let anchor = self.securityGuard.focusedCaretScreenRect()
                self.popover.showToast("Отменено: «\(last.original)» (запомнено)", anchorRect: anchor)
            }
        }
    }

    private func runInjection(
        generation gen: UInt64,
        targetPID: pid_t,
        deleteCount: Int,
        insertText: String,
        settleMicroseconds: useconds_t,
        onDone: (@Sendable (Bool) -> Void)? = nil
    ) {
        let shared = self.sharedState

        injector.replaceSuffix(
            deleteCount: deleteCount,
            insertText: insertText,
            settleMicroseconds: settleMicroseconds,
            shouldContinue: {
                shared.canContinueInjection(generation: gen, expectedPID: targetPID)
                    && !IsSecureEventInputEnabled()
            },
            popDeferredOrFinish: {
                shared.popDeferredOrFinish(generation: gen)
            },
            onComplete: { [weak self] completed in
                if !completed {
                    let wasCurrent = shared.finishAborted(generation: gen)
                    if wasCurrent {
                        Task { @MainActor [weak self] in
                            self?.resetState(dismissPopover: true)
                        }
                    }
                }
                onDone?(completed)
            }
        )
    }

    private func armUndoWindow(seconds: Double, appPID: pid_t) {
        undoExpiryTask?.cancel()
        lastCorrectionAppPID = appPID
        sharedState.setUndoState(active: true, appPID: appPID)
        undoExpiryTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(Int(seconds * 1000)))
            guard !Task.isCancelled else { return }
            self?.clearUndoWindow()
        }
    }

    private func clearUndoWindow() {
        undoExpiryTask?.cancel()
        undoExpiryTask = nil
        lastCorrectionAppPID = nil
        sharedState.setUndoState(active: false)
        wordCorrector.clearLastCorrection()
    }

    private func scheduleSentenceCheck(delaySeconds: Double) {
        debounceTask?.cancel()
        debounceTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(Int(delaySeconds * 1000)))
            guard !Task.isCancelled, let self else { return }
            let cfg = self.configProvider()
            guard cfg.sentenceCheckEnabled, self.securityGuard.shouldMonitor(config: cfg) else { return }
            guard let pending = self.typingBuffer.currentPendingSentence() else { return }
            if pending.sentence == self.lastCheckedSentence { return }
            self.triggerSentenceCheck(pending)
        }
    }

    private func triggerSentenceCheck(_ pending: PendingSentence) {
        lastCheckedSentence = pending.sentence
        guard let capturedPID = securityGuard.frontmostApp()?.processIdentifier else { return }
        let expectedTrackable = typingBuffer.activeTrackableText

        sentenceClient.checkSentence(pending.sentence) { [weak self] resp in
            guard let self, let resp, resp.changed else { return }
            let cfg = self.configProvider()
            guard cfg.sentenceCheckEnabled,
                  self.securityGuard.frontmostApp()?.processIdentifier == capturedPID,
                  self.securityGuard.shouldMonitor(config: cfg)
            else { return }

            let currentPending = self.typingBuffer.currentPendingSentence()
            let matchedPending: PendingSentence
            let stillMatches: Bool
            if let cur = currentPending, cur.sentence == pending.sentence,
               self.typingBuffer.activeTrackableText.hasSuffix(cur.fullTrackedSuffix)
            {
                matchedPending = cur
                stillMatches = true
            } else if self.typingBuffer.activeTrackableText == expectedTrackable,
                      self.typingBuffer.activeTrackableText.hasSuffix(pending.fullTrackedSuffix)
            {
                matchedPending = pending
                stillMatches = true
            } else {
                // User already edited or moved past this sentence while model was running -> suppress stale popover.
                return
            }

            self.activeSentencePending = matchedPending
            self.activeSentenceAppPID = capturedPID

            let anchor = self.securityGuard.focusedCaretScreenRect()
            let suggestion = SentencePopoverController.Suggestion(
                original: matchedPending.sentence,
                trailingWhitespace: matchedPending.trailingWhitespace,
                corrected: resp.corrected,
                latencyMs: resp.latency_ms,
                canReplaceInPlace: stillMatches
            )
            self.popover.showSuggestion(suggestion, anchorRect: anchor)
            self.sharedState.setPopoverState(visible: true, appPID: capturedPID)
        }
    }

    private func applySentenceSuggestion(
        _ suggestion: SentencePopoverController.Suggestion,
        preArmedGeneration: UInt64?
    ) {
        sharedState.setPopoverState(visible: false)
        let cfg = configProvider()
        guard suggestion.canReplaceInPlace,
              let pending = activeSentencePending,
              let expectedPID = activeSentenceAppPID,
              pending.sentence == suggestion.original,
              securityGuard.frontmostApp()?.processIdentifier == expectedPID,
              securityGuard.shouldMonitor(config: cfg),
              typingBuffer.activeTrackableText.hasSuffix(pending.fullTrackedSuffix)
        else {
            if let gen = preArmedGeneration {
                sharedState.finishAborted(generation: gen)
            }
            NSPasteboard.general.clearContents()
            NSPasteboard.general.setString(suggestion.corrected, forType: .string)
            popover.showToast("Курсор сместился — скопировано в буфер", anchorRect: nil)
            return
        }

        let gen: UInt64
        if let pre = preArmedGeneration {
            gen = pre
        } else if let armed = sharedState.beginInjection(targetPID: expectedPID) {
            gen = armed
        } else {
            return
        }

        clearUndoWindow()
        typingBuffer.applySentenceReplacement(
            originalSuffix: pending.fullTrackedSuffix,
            replacementWithTrailing: suggestion.correctedWithTrailing
        )
        lastCheckedSentence = suggestion.corrected
        activeSentencePending = nil

        runInjection(
            generation: gen,
            targetPID: expectedPID,
            deleteCount: pending.fullTrackedSuffix.count,
            insertText: suggestion.correctedWithTrailing,
            settleMicroseconds: 2_000
        )
    }

    private func handleGlobalMouseClick() {
        let mouseLocation = NSEvent.mouseLocation
        if popover.isVisible && popover.panelFrame.contains(mouseLocation) {
            return
        }
        resetState(dismissPopover: true)
    }

    func resetState(dismissPopover: Bool) {
        sharedState.abortCurrentInjection()
        debounceTask?.cancel()
        sentenceClient.cancelPending()
        clearUndoWindow()
        typingBuffer.reset()
        activeSentencePending = nil
        activeSentenceAppPID = nil
        lastCheckedSentence = nil
        if dismissPopover {
            popover.hide()
            sharedState.setPopoverState(visible: false)
        }
    }
}
