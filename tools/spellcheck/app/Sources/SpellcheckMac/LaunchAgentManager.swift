import Foundation

enum LaunchAgentManager {
    static let label = "local.dots.spellcheck"

    static var plistURL: URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/LaunchAgents", isDirectory: true)
            .appendingPathComponent("\(label).plist")
    }

    static var isEnabled: Bool {
        FileManager.default.fileExists(atPath: plistURL.path)
    }

    static func setEnabled(_ enabled: Bool, executablePath: String? = nil) throws {
        let fm = FileManager.default
        // Never follow a symlink in LaunchAgents.
        if let attrs = try? fm.attributesOfItem(atPath: plistURL.path),
           (attrs[.type] as? FileAttributeType) == .typeSymbolicLink
        {
            return
        }

        if enabled {
            let dir = plistURL.deletingLastPathComponent()
            try fm.createDirectory(at: dir, withIntermediateDirectories: true)

            let execPath = executablePath
                ?? Bundle.main.executablePath
                ?? "/Applications/SpellcheckMac.app/Contents/MacOS/SpellcheckMac"

            let plist: [String: Any] = [
                "Label": label,
                "ProgramArguments": [execPath],
                "RunAtLoad": true,
                "KeepAlive": false,
                "ProcessType": "Interactive",
                "LimitLoadToSessionType": "Aqua",
            ]
            let data = try PropertyListSerialization.data(
                fromPropertyList: plist,
                format: .xml,
                options: 0
            )
            try data.write(to: plistURL, options: .atomic)
        } else if fm.fileExists(atPath: plistURL.path) {
            try fm.removeItem(at: plistURL)
        }
    }
}
