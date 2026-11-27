import Foundation

public protocol SpellCheckerBackend: Sendable {
    /// Returns the single top correction if the entire word is misspelled, or nil otherwise.
    func correction(forWord word: String, language: String) -> String?
}

public struct AutoCorrection: Equatable, Sendable {
    public let original: String
    public let replacement: String
    public let delimiter: String
    public let timestamp: Date

    public init(original: String, replacement: String, delimiter: String, timestamp: Date) {
        self.original = original
        self.replacement = replacement
        self.delimiter = delimiter
        self.timestamp = timestamp
    }
}

public final class WordCorrector {
    private let backend: SpellCheckerBackend
    public var language: String
    public var minWordLength: Int
    public var ignoredWords: Set<String>
    public private(set) var lastCorrection: AutoCorrection?

    public init(
        backend: SpellCheckerBackend,
        language: String = "ru",
        minWordLength: Int = 4,
        ignoredWords: Set<String> = []
    ) {
        self.backend = backend
        self.language = language
        self.minWordLength = minWordLength
        self.ignoredWords = Set(ignoredWords.map { $0.lowercased() })
    }

    public func suggestAutoCorrection(for word: String) -> String? {
        guard isEligibleWord(word) else { return nil }
        let lower = word.lowercased()
        if ignoredWords.contains(lower) { return nil }

        guard let rawSuggestion = backend.correction(forWord: word, language: language) else {
            return nil
        }
        let suggestion = rawSuggestion.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !suggestion.isEmpty,
              !suggestion.contains(where: { $0.isWhitespace }),
              isPureCyrillicWord(suggestion)
        else { return nil }

        let suggLower = suggestion.lowercased()
        if suggLower == lower { return nil }

        // Never auto-replace solely for е <-> ё differences.
        if normalizeYo(lower) == normalizeYo(suggLower) { return nil }

        // Reject proper-noun (capitalized) suggestion when user typed all-lowercase.
        let isUserAllLower = word == lower
        let isSuggestionCapitalized = suggestion.first?.isUppercase == true
        if isUserAllLower && isSuggestionCapitalized { return nil }

        // Strict Damerau-Levenshtein distance budget to avoid aggressive wrong replacements.
        let dist = Self.damerauLevenshtein(lower, suggLower)
        let maxDist = word.count <= 5 ? 1 : 2
        let maxRatio = 0.34
        guard dist >= 1,
              dist <= maxDist,
              Double(dist) / Double(word.count) <= maxRatio
        else { return nil }

        return matchCase(of: word, onto: suggestion)
    }

    public func recordCorrection(
        original: String,
        replacement: String,
        delimiter: String,
        now: Date = Date()
    ) {
        lastCorrection = AutoCorrection(
            original: original,
            replacement: replacement,
            delimiter: delimiter,
            timestamp: now
        )
    }

    public func clearLastCorrection() {
        lastCorrection = nil
    }

    /// If the user hits Backspace shortly after an auto-correction, consume it and ignore the original word for this session.
    public func consumeUndoIfRecent(now: Date = Date(), maxAge: TimeInterval = 5.0) -> AutoCorrection? {
        guard let last = lastCorrection else { return nil }
        lastCorrection = nil
        guard now.timeIntervalSince(last.timestamp) <= maxAge else { return nil }
        ignoredWords.insert(last.original.lowercased())
        return last
    }

    public func ignoreWord(_ word: String) {
        ignoredWords.insert(word.lowercased())
    }

    // MARK: - Helpers

    public func isEligibleWord(_ word: String) -> Bool {
        guard word.count >= minWordLength else { return false }
        guard isPureCyrillicWord(word) else { return false }
        // Skip ALL-CAPS acronyms (МГУ, СССР, НДС).
        if word == word.uppercased() && word != word.lowercased() {
            return false
        }
        // Skip CamelCase / internal uppercase letters.
        let tail = word.dropFirst()
        if tail.contains(where: { $0.isUppercase }) {
            return false
        }
        return true
    }

    public func isPureCyrillicWord(_ word: String) -> Bool {
        guard !word.isEmpty else { return false }
        if word.first == "-" || word.last == "-" { return false }
        var hyphenCount = 0
        for scalar in word.unicodeScalars {
            if scalar == "-" {
                hyphenCount += 1
                if hyphenCount > 1 { return false }
                continue
            }
            if !Self.isCyrillicLetter(scalar) {
                return false
            }
        }
        return true
    }

    public static func isCyrillicLetter(_ scalar: UnicodeScalar) -> Bool {
        let v = scalar.value
        return (v >= 0x0410 && v <= 0x044F) || v == 0x0401 || v == 0x0451
    }

    private func normalizeYo(_ s: String) -> String {
        s.replacingOccurrences(of: "ё", with: "е")
    }

    private func matchCase(of original: String, onto suggestion: String) -> String {
        guard let first = original.first else { return suggestion }
        if first.isUppercase {
            let sLower = suggestion.lowercased()
            return sLower.prefix(1).uppercased() + sLower.dropFirst()
        }
        return suggestion.lowercased()
    }

    public static func damerauLevenshtein(_ a: String, _ b: String) -> Int {
        let s = Array(a)
        let t = Array(b)
        let n = s.count
        let m = t.count
        if n == 0 { return m }
        if m == 0 { return n }

        var d = Array(repeating: Array(repeating: 0, count: m + 1), count: n + 1)
        for i in 0...n { d[i][0] = i }
        for j in 0...m { d[0][j] = j }

        for i in 1...n {
            for j in 1...m {
                let cost = s[i - 1] == t[j - 1] ? 0 : 1
                var best = min(
                    d[i - 1][j] + 1,       // deletion
                    d[i][j - 1] + 1,       // insertion
                    d[i - 1][j - 1] + cost // substitution
                )
                if i > 1 && j > 1 && s[i - 1] == t[j - 2] && s[i - 2] == t[j - 1] {
                    best = min(best, d[i - 2][j - 2] + 1) // transposition
                }
                d[i][j] = best
            }
        }
        return d[n][m]
    }
}
