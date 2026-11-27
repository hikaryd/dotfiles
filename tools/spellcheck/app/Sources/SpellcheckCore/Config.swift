import Foundation

public struct AppConfig: Codable, Sendable, Equatable {
    public var wordAutocorrectEnabled: Bool
    public var sentenceCheckEnabled: Bool
    public var serverPort: Int
    public var debounceSeconds: Double
    public var excludedBundleIDs: [String]
    public var excludedBundlePrefixes: [String]
    /// Explicit user overrides that re-enable apps matching `excludedBundlePrefixes`.
    public var includedBundleIDs: [String]
    public var userIgnoredWords: [String]

    public static let defaultExcludedBundleIDs: [String] = [
        // Terminals
        "com.apple.Terminal",
        "com.googlecode.iterm2",
        "dev.warp.Warp-Stable",
        "com.mitchellh.ghostty",
        "net.kovidgoyal.kitty",
        "io.alacritty",
        "org.alacritty",
        "co.zeit.hyper",
        "com.github.wez.wezterm",
        "net.sourceforge.iTerm",
        // IDEs & code editors
        "com.microsoft.VSCode",
        "com.microsoft.VSCodeInsiders",
        "com.todesktop.230313mzl4w4u92", // Cursor
        "com.exafunction.windsurf",
        "dev.zed.Zed",
        "com.apple.dt.Xcode",
        "com.panic.Nova",
        "com.sublimetext.4",
        "com.sublimetext.3",
        "com.github.atom",
        "org.gnu.Emacs",
        "com.qvacua.VimR",
        // Password managers & security
        "com.apple.keychainaccess",
        "com.apple.Passwords",
        "com.bitwarden.desktop",
        "org.keepassxc.keepassxc",
        "com.dashlane.Dashlane",
        "com.lastpass.LastPass",
        "com.1password.1password",
        "com.agilebits.onepassword7",
        // System launchers / spotlight
        "com.apple.Spotlight",
        "com.raycast.macos",
        "com.runningwithcrayons.Alfred",
    ]

    public static let defaultExcludedBundlePrefixes: [String] = [
        "com.jetbrains.",
        "com.1password.",
        "com.agilebits.",
        "com.sublimetext.",
    ]

    public static let `default` = AppConfig(
        wordAutocorrectEnabled: true,
        sentenceCheckEnabled: true,
        serverPort: 18765,
        debounceSeconds: 1.1,
        excludedBundleIDs: defaultExcludedBundleIDs,
        excludedBundlePrefixes: defaultExcludedBundlePrefixes,
        includedBundleIDs: [],
        userIgnoredWords: []
    )

    public init(
        wordAutocorrectEnabled: Bool,
        sentenceCheckEnabled: Bool,
        serverPort: Int,
        debounceSeconds: Double,
        excludedBundleIDs: [String],
        excludedBundlePrefixes: [String],
        includedBundleIDs: [String] = [],
        userIgnoredWords: [String]
    ) {
        self.wordAutocorrectEnabled = wordAutocorrectEnabled
        self.sentenceCheckEnabled = sentenceCheckEnabled
        self.serverPort = serverPort
        self.debounceSeconds = debounceSeconds
        self.excludedBundleIDs = excludedBundleIDs
        self.excludedBundlePrefixes = excludedBundlePrefixes
        self.includedBundleIDs = includedBundleIDs
        self.userIgnoredWords = userIgnoredWords
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let def = AppConfig.default
        wordAutocorrectEnabled = try c.decodeIfPresent(Bool.self, forKey: .wordAutocorrectEnabled) ?? def.wordAutocorrectEnabled
        sentenceCheckEnabled = try c.decodeIfPresent(Bool.self, forKey: .sentenceCheckEnabled) ?? def.sentenceCheckEnabled
        serverPort = try c.decodeIfPresent(Int.self, forKey: .serverPort) ?? def.serverPort
        debounceSeconds = try c.decodeIfPresent(Double.self, forKey: .debounceSeconds) ?? def.debounceSeconds
        excludedBundleIDs = try c.decodeIfPresent([String].self, forKey: .excludedBundleIDs) ?? def.excludedBundleIDs
        excludedBundlePrefixes = try c.decodeIfPresent([String].self, forKey: .excludedBundlePrefixes) ?? def.excludedBundlePrefixes
        includedBundleIDs = try c.decodeIfPresent([String].self, forKey: .includedBundleIDs) ?? []
        userIgnoredWords = try c.decodeIfPresent([String].self, forKey: .userIgnoredWords) ?? []
    }

    public func isExcluded(bundleID: String?) -> Bool {
        guard let id = bundleID, !id.isEmpty else { return false }
        if includedBundleIDs.contains(id) { return false }
        if excludedBundleIDs.contains(id) || Self.defaultExcludedBundleIDs.contains(id) { return true }
        return excludedBundlePrefixes.contains { id.hasPrefix($0) }
    }

    public mutating func toggleExclusion(bundleID: String) -> Bool {
        let isDefaultExcluded = Self.defaultExcludedBundleIDs.contains(bundleID)
            || excludedBundlePrefixes.contains(where: { bundleID.hasPrefix($0) })
        if isExcluded(bundleID: bundleID) {
            excludedBundleIDs.removeAll { $0 == bundleID }
            if isDefaultExcluded && !includedBundleIDs.contains(bundleID) {
                includedBundleIDs.append(bundleID)
            }
            return false
        } else {
            includedBundleIDs.removeAll { $0 == bundleID }
            if !isDefaultExcluded && !excludedBundleIDs.contains(bundleID) {
                excludedBundleIDs.append(bundleID)
            }
            return true
        }
    }
}

public enum ConfigStore {
    public static var configURL: URL {
        let appSupport = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first!
        return appSupport
            .appendingPathComponent("SpellcheckMac", isDirectory: true)
            .appendingPathComponent("config.json")
    }

    public static func load(from url: URL = configURL) -> AppConfig {
        guard let data = try? Data(contentsOf: url),
              let cfg = try? JSONDecoder().decode(AppConfig.self, from: data)
        else {
            return .default
        }
        return cfg
    }

    public static func save(_ config: AppConfig, to url: URL = configURL) {
        let dir = url.deletingLastPathComponent()
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let enc = JSONEncoder()
        enc.outputFormatting = [.prettyPrinted, .sortedKeys]
        if let data = try? enc.encode(config) {
            try? data.write(to: url, options: .atomic)
        }
    }
}
