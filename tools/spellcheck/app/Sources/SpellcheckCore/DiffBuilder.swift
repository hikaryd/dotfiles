import Foundation

public enum DiffSegmentKind: Equatable, Sendable {
    case equal
    case deleted
    case inserted
}

public struct DiffSegment: Equatable, Sendable {
    public let kind: DiffSegmentKind
    public let text: String

    public init(kind: DiffSegmentKind, text: String) {
        self.kind = kind
        self.text = text
    }
}

public enum DiffBuilder {
    /// Splits text into word / punctuation / whitespace tokens and computes LCS diff.
    public static func diff(original: String, corrected: String) -> [DiffSegment] {
        let a = tokenize(original)
        let b = tokenize(corrected)
        let n = a.count
        let m = b.count
        if n == 0 && m == 0 { return [] }

        var lcs = Array(repeating: Array(repeating: 0, count: m + 1), count: n + 1)
        if n > 0 && m > 0 {
            for i in (0..<n).reversed() {
                for j in (0..<m).reversed() {
                    if a[i] == b[j] {
                        lcs[i][j] = lcs[i + 1][j + 1] + 1
                    } else {
                        lcs[i][j] = max(lcs[i + 1][j], lcs[i][j + 1])
                    }
                }
            }
        }

        var raw: [DiffSegment] = []
        var i = 0
        var j = 0
        while i < n || j < m {
            if i < n && j < m && a[i] == b[j] {
                raw.append(DiffSegment(kind: .equal, text: a[i]))
                i += 1
                j += 1
            } else if j < m && (i == n || lcs[i][j + 1] >= lcs[i + 1][j]) {
                raw.append(DiffSegment(kind: .inserted, text: b[j]))
                j += 1
            } else {
                raw.append(DiffSegment(kind: .deleted, text: a[i]))
                i += 1
            }
        }

        // Merge adjacent segments of identical kind.
        var merged: [DiffSegment] = []
        for seg in raw {
            if let last = merged.last, last.kind == seg.kind {
                merged[merged.count - 1] = DiffSegment(kind: seg.kind, text: last.text + seg.text)
            } else {
                merged.append(seg)
            }
        }
        return merged
    }

    private static func tokenize(_ s: String) -> [String] {
        var tokens: [String] = []
        var buf = ""
        enum Class { case word, space, punct }
        var cur: Class?

        func flush() {
            if !buf.isEmpty {
                tokens.append(buf)
                buf = ""
            }
        }

        for ch in s {
            let cls: Class = ch.isLetter || ch.isNumber || ch == "-" ? .word
                : ch.isWhitespace ? .space
                : .punct
            if cls == .punct {
                flush()
                tokens.append(String(ch))
                cur = nil
            } else if cls == cur {
                buf.append(ch)
            } else {
                flush()
                buf.append(ch)
                cur = cls
            }
        }
        flush()
        return tokens
    }
}
