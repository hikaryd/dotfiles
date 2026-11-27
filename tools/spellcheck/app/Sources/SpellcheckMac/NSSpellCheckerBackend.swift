import AppKit
import SpellcheckCore

final class NSSpellCheckerBackend: SpellCheckerBackend, @unchecked Sendable {
    private let documentTag: Int

    init() {
        self.documentTag = NSSpellChecker.uniqueSpellDocumentTag()
    }

    func correction(forWord word: String, language: String) -> String? {
        let sc = NSSpellChecker.shared
        let utf16Count = word.utf16.count
        let misspelled = sc.checkSpelling(
            of: word,
            startingAt: 0,
            language: language,
            wrap: false,
            inSpellDocumentWithTag: documentTag,
            wordCount: nil
        )
        guard misspelled.location == 0 && misspelled.length == utf16Count else {
            return nil
        }
        return sc.correction(
            forWordRange: misspelled,
            in: word,
            language: language,
            inSpellDocumentWithTag: documentTag
        )
    }
}
