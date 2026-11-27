import Foundation

public enum UTF16Chunker {
    /// Splits `text` into UTF-16 code unit slices of at most `maxChunkSize` without ever splitting a surrogate pair.
    public static func chunk(_ text: String, maxChunkSize: Int = 12) -> [[UInt16]] {
        let limit = max(2, maxChunkSize)
        let units = Array(text.utf16)
        guard !units.isEmpty else { return [] }

        var result: [[UInt16]] = []
        var offset = 0
        while offset < units.count {
            var end = min(offset + limit, units.count)
            // If the last code unit in the slice is a UTF-16 lead surrogate (0xD800...0xDBFF),
            // do not split it from its trailing low surrogate.
            if end < units.count && UTF16.isLeadSurrogate(units[end - 1]) {
                end -= 1
            }
            if end <= offset {
                // Defensive fallback for malformed lone lead surrogate at boundary.
                end = min(offset + 2, units.count)
            }
            result.append(Array(units[offset..<end]))
            offset = end
        }
        return result
    }
}
