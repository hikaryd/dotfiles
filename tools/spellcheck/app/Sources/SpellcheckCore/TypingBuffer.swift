import Foundation

public struct PendingSentence: Equatable, Sendable {
    /// Trimmed sentence text sent to the model.
    public let sentence: String
    /// Exact trailing whitespace after `sentence` still present in the editor (for accurate in-place replacement).
    public let trailingWhitespace: String

    public var fullTrackedSuffix: String { sentence + trailingWhitespace }

    public init(sentence: String, trailingWhitespace: String) {
        self.sentence = sentence
        self.trailingWhitespace = trailingWhitespace
    }
}

public struct TypingEventOutcome: Equatable, Sendable {
    public let completedWord: String?
    public let delimiter: String?
    /// Short-delay settle trigger after sentence-ending punctuation (handles `...` and `?!` cleanly).
    public let shouldScheduleQuickSentenceCheck: Bool
    /// Standard idle pause debounce trigger mid-sentence.
    public let shouldScheduleIdleDebounce: Bool
    /// True if user is only extending sentence-ending punctuation (e.g. second dot in `...`), so don't cancel prematurely.
    public let isPunctuationContinuation: Bool

    public static let none = TypingEventOutcome(
        completedWord: nil,
        delimiter: nil,
        shouldScheduleQuickSentenceCheck: false,
        shouldScheduleIdleDebounce: false,
        isPunctuationContinuation: false
    )
}

public final class TypingBuffer {
    public private(set) var currentWord: String = ""
    public private(set) var currentSentence: String = ""
    /// Tracks exact characters currently at the end of the active sentence in the focused field.
    public private(set) var activeTrackableText: String = ""
    public private(set) var wordDirty: Bool = false
    /// Set after `.`/`!`/`?`; once a new non-whitespace, non-ender character arrives, starts a fresh sentence buffer.
    public private(set) var sentenceTerminated: Bool = false

    private static let sentenceEnders: Set<Character> = [".", "!", "?"]
    private static let wordDelimiters: Set<Character> = [
        " ", "\t", ",", ";", ":", ".", "!", "?", ")", "(", "\"", "«", "»", "—", "–"
    ]

    public init() {}

    public func reset() {
        currentWord = ""
        currentSentence = ""
        activeTrackableText = ""
        wordDirty = false
        sentenceTerminated = false
    }

    public func handleTypedString(_ text: String) -> TypingEventOutcome {
        guard !text.isEmpty else { return .none }
        guard text.count == 1, let ch = text.first else {
            reset()
            return .none
        }

        if ch == "\n" || ch == "\r" {
            reset()
            return .none
        }

        // Consecutive sentence-enders (`...`, `?!`): extend current sentence without clearing or cancelling.
        if sentenceTerminated && Self.sentenceEnders.contains(ch) {
            currentWord = ""
            wordDirty = false
            currentSentence.append(ch)
            activeTrackableText.append(ch)
            return TypingEventOutcome(
                completedWord: nil,
                delimiter: String(ch),
                shouldScheduleQuickSentenceCheck: currentPendingSentence() != nil,
                shouldScheduleIdleDebounce: false,
                isPunctuationContinuation: true
            )
        }

        // Trailing space after `.` / `!` / `?`: keep terminated sentence intact so Apply still works,
        // while tracking the trailing space cleanly.
        if sentenceTerminated && (ch == " " || ch == "\t") {
            currentWord = ""
            wordDirty = false
            currentSentence.append(ch)
            activeTrackableText.append(ch)
            return TypingEventOutcome(
                completedWord: nil,
                delimiter: String(ch),
                shouldScheduleQuickSentenceCheck: false,
                shouldScheduleIdleDebounce: false,
                isPunctuationContinuation: true
            )
        }

        // User started typing a brand new sentence after `.` / `. ` -> reset trackable prefix cleanly.
        if sentenceTerminated {
            currentSentence = ""
            activeTrackableText = ""
            currentWord = ""
            wordDirty = false
            sentenceTerminated = false
        }

        if Self.wordDelimiters.contains(ch) {
            let wordToCheck: String? = (!wordDirty && !currentWord.isEmpty) ? currentWord : nil
            currentWord = ""
            wordDirty = false

            currentSentence.append(ch)
            activeTrackableText.append(ch)

            if Self.sentenceEnders.contains(ch) {
                sentenceTerminated = true
                return TypingEventOutcome(
                    completedWord: wordToCheck,
                    delimiter: String(ch),
                    shouldScheduleQuickSentenceCheck: currentPendingSentence() != nil,
                    shouldScheduleIdleDebounce: false,
                    isPunctuationContinuation: false
                )
            }

            return TypingEventOutcome(
                completedWord: wordToCheck,
                delimiter: String(ch),
                shouldScheduleQuickSentenceCheck: false,
                shouldScheduleIdleDebounce: currentPendingSentence() != nil,
                isPunctuationContinuation: false
            )
        }

        // Regular character
        let isWordChar = ch.unicodeScalars.allSatisfy {
            WordCorrector.isCyrillicLetter($0) || $0 == "-"
        }
        if isWordChar {
            currentWord.append(ch)
        } else {
            currentWord = ""
            wordDirty = true
        }

        currentSentence.append(ch)
        activeTrackableText.append(ch)

        if currentSentence.count > 350 {
            reset()
            return .none
        }

        return TypingEventOutcome(
            completedWord: nil,
            delimiter: nil,
            shouldScheduleQuickSentenceCheck: false,
            shouldScheduleIdleDebounce: currentPendingSentence() != nil,
            isPunctuationContinuation: false
        )
    }

    public func handleBackspace() {
        if !currentWord.isEmpty {
            currentWord.removeLast()
        } else {
            wordDirty = true
        }
        if !currentSentence.isEmpty {
            currentSentence.removeLast()
        }
        if !activeTrackableText.isEmpty {
            activeTrackableText.removeLast()
        }
        // Recompute whether sentence is still terminated after deleting last char (e.g., Backspace after `.`).
        let trimmedRight = currentSentence.replacingOccurrences(of: #"[ \t]+$"#, with: "", options: .regularExpression)
        if let last = trimmedRight.last, Self.sentenceEnders.contains(last) {
            sentenceTerminated = true
        } else {
            sentenceTerminated = false
        }
    }

    public func applyWordReplacement(original: String, replacement: String, delimiter: String) {
        let oldSuffix = original + delimiter
        let newSuffix = replacement + delimiter
        if currentSentence.hasSuffix(oldSuffix) {
            currentSentence.removeLast(oldSuffix.count)
            currentSentence.append(newSuffix)
        }
        if activeTrackableText.hasSuffix(oldSuffix) {
            activeTrackableText.removeLast(oldSuffix.count)
            activeTrackableText.append(newSuffix)
        }
    }

    public func applyUndoWordReplacement(replacement: String, original: String, delimiter: String) {
        let currentSuffix = replacement + delimiter
        let restoredSuffix = original + delimiter
        if currentSentence.hasSuffix(currentSuffix) {
            currentSentence.removeLast(currentSuffix.count)
            currentSentence.append(restoredSuffix)
        }
        if activeTrackableText.hasSuffix(currentSuffix) {
            activeTrackableText.removeLast(currentSuffix.count)
            activeTrackableText.append(restoredSuffix)
        }
    }

    public func applySentenceReplacement(originalSuffix: String, replacementWithTrailing: String) {
        if activeTrackableText.hasSuffix(originalSuffix) {
            activeTrackableText.removeLast(originalSuffix.count)
            activeTrackableText.append(replacementWithTrailing)
        } else {
            activeTrackableText = replacementWithTrailing
        }
        if currentSentence.hasSuffix(originalSuffix) {
            currentSentence.removeLast(originalSuffix.count)
            currentSentence.append(replacementWithTrailing)
        }
        currentWord = ""
        wordDirty = false
    }

    /// Returns the trimmed checkable sentence plus exact trailing spaces still in the editor.
    public func currentPendingSentence() -> PendingSentence? {
        guard !currentSentence.isEmpty else { return nil }
        var endIdx = currentSentence.endIndex
        while endIdx > currentSentence.startIndex {
            let prev = currentSentence.index(before: endIdx)
            let c = currentSentence[prev]
            if c == " " || c == "\t" {
                endIdx = prev
            } else {
                break
            }
        }
        let trailing = String(currentSentence[endIdx...])
        let withoutTrailing = String(currentSentence[..<endIdx])
        let trimmed = withoutTrailing.drop(while: { $0 == " " || $0 == "\t" })
        let sentence = String(trimmed)
        guard Self.isCheckableSentence(sentence) else { return nil }
        return PendingSentence(sentence: sentence, trailingWhitespace: trailing)
    }

    public static func isCheckableSentence(_ text: String) -> Bool {
        guard text.count >= 8 else { return false }
        let words = text.split { !$0.isLetter }
        let cyrillicWords = words.filter { w in
            w.unicodeScalars.contains(where: WordCorrector.isCyrillicLetter)
        }
        return cyrillicWords.count >= 2
    }
}
