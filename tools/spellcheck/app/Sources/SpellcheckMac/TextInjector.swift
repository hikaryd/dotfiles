import CoreGraphics
import Foundation
import SpellcheckCore

struct SendableCGEvent: @unchecked Sendable {
    let event: CGEvent
}

enum PopDeferredResult: Sendable {
    case batch([SendableCGEvent])
    case finishedCleanly
    case abortedOrSuperseded
}

final class TextInjector: Sendable {
    /// Magic tag attached to synthetic Backspace / replacement events (ignored by TypingMonitor).
    static let selfEventTag: Int64 = 0x5350454C4C // "SPELL"
    /// Magic tag attached to replayed user keystrokes captured during injection (forwarded to editor + TypingMonitor).
    static let replayEventTag: Int64 = 0x5350454C4D // "SPELM"

    private let queue = DispatchQueue(label: "com.spellcheckmac.textinjector", qos: .userInteractive)

    /// Deletes `deleteCount` characters via synthetic Backspaces and types `insertText` via surrogate-safe Unicode chunks.
    /// Checks `shouldContinue()` before every posted event and after post-chunk settles.
    /// Replays any user keystrokes deferred during injection in strict FIFO order while `isInjecting` remains armed,
    /// atomically clearing `isInjecting` via `popDeferredOrFinish()` only once the deferred queue is empty.
    func replaceSuffix(
        deleteCount: Int,
        insertText: String,
        settleMicroseconds: useconds_t = 5_000,
        shouldContinue: @escaping @Sendable () -> Bool,
        popDeferredOrFinish: @escaping @Sendable () -> PopDeferredResult,
        onComplete: @escaping @Sendable (Bool) -> Void
    ) {
        let safeText = insertText
            .replacingOccurrences(of: "\r\n", with: " ")
            .replacingOccurrences(of: "\n", with: " ")
            .replacingOccurrences(of: "\r", with: " ")

        let chunks = UTF16Chunker.chunk(safeText, maxChunkSize: 12)

        queue.async {
            guard let source = CGEventSource(stateID: .privateState) else {
                onComplete(false)
                return
            }

            if settleMicroseconds > 0 {
                usleep(settleMicroseconds)
            }

            guard shouldContinue() else {
                onComplete(false)
                return
            }

            let backspaceKeyCode: CGKeyCode = 51
            for _ in 0..<max(0, deleteCount) {
                guard shouldContinue() else {
                    onComplete(false)
                    return
                }
                Self.postKey(source: source, keyCode: backspaceKeyCode, keyDown: true)
                Self.postKey(source: source, keyCode: backspaceKeyCode, keyDown: false)
                usleep(800)
            }

            if deleteCount > 0 {
                usleep(3_000)
            }

            for rawChunk in chunks {
                guard shouldContinue() else {
                    onComplete(false)
                    return
                }
                var chunk = rawChunk
                Self.postUnicodeChunk(source: source, chunk: &chunk)
                usleep(1_200)
            }

            usleep(2_000)

            // Re-verify after final settle sleep and drain deferred user keystrokes in strict FIFO order
            // while `isInjecting` stays armed so live keys cannot overtake buffered keys.
            while true {
                guard shouldContinue() else {
                    onComplete(false)
                    return
                }
                switch popDeferredOrFinish() {
                case .finishedCleanly:
                    onComplete(true)
                    return
                case .abortedOrSuperseded:
                    onComplete(false)
                    return
                case .batch(let events):
                    for item in events {
                        guard shouldContinue() else {
                            onComplete(false)
                            return
                        }
                        item.event.setIntegerValueField(.eventSourceUserData, value: Self.replayEventTag)
                        item.event.post(tap: .cghidEventTap)
                        usleep(1_000)
                    }
                }
            }
        }
    }

    private static func postKey(source: CGEventSource, keyCode: CGKeyCode, keyDown: Bool) {
        guard let ev = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: keyDown) else {
            return
        }
        ev.flags = [] // Clear Shift/Option/Cmd so Backspace never becomes Option+Backspace.
        ev.setIntegerValueField(.eventSourceUserData, value: selfEventTag)
        ev.post(tap: .cghidEventTap)
    }

    private static func postUnicodeChunk(source: CGEventSource, chunk: inout [UniChar]) {
        guard let down = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: true),
              let up = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: false)
        else { return }

        down.flags = []
        up.flags = []
        down.setIntegerValueField(.eventSourceUserData, value: selfEventTag)
        up.setIntegerValueField(.eventSourceUserData, value: selfEventTag)
        chunk.withUnsafeBufferPointer { buf in
            guard let base = buf.baseAddress else { return }
            down.keyboardSetUnicodeString(stringLength: buf.count, unicodeString: base)
            up.keyboardSetUnicodeString(stringLength: buf.count, unicodeString: base)
        }
        down.post(tap: .cghidEventTap)
        up.post(tap: .cghidEventTap)
    }
}
