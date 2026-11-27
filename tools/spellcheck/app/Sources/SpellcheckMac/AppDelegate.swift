import AppKit
import ApplicationServices
import SpellcheckCore

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var config: AppConfig = ConfigStore.load()
    private let securityGuard = SecurityGuard()
    private lazy var wordCorrector = WordCorrector(
        backend: NSSpellCheckerBackend(),
        language: "ru",
        minWordLength: 4,
        ignoredWords: Set(config.userIgnoredWords)
    )
    private lazy var sentenceClient = SentenceClient(port: config.serverPort)
    private let popover = SentencePopoverController()
    private lazy var monitor = TypingMonitor(
        securityGuard: securityGuard,
        wordCorrector: wordCorrector,
        sentenceClient: sentenceClient,
        popover: popover,
        configProvider: { [weak self] in self?.config ?? .default }
    )

    private var statusItem: NSStatusItem!
    private var serverState: ServerState = .starting
    private var permissionPollTask: Task<Void, Never>?
    private var signalSources: [DispatchSourceSignal] = []
    private var lastNonSelfBundleID: String?
    private var lastNonSelfAppName: String?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        installSignalHandlers()
        setupStatusBar()
        observeFrontmostApp()
        startServer()
        startMonitoringOrRequestPermission()
    }

    func applicationWillTerminate(_ notification: Notification) {
        permissionPollTask?.cancel()
        signalSources.forEach { $0.cancel() }
        signalSources.removeAll()
        monitor.stop()
        sentenceClient.stopSubprocess()
    }

    private func installSignalHandlers() {
        for sig in [SIGTERM, SIGINT] {
            signal(sig, SIG_IGN)
            let src = DispatchSource.makeSignalSource(signal: sig, queue: .main)
            src.setEventHandler {
                NSApp.terminate(nil)
            }
            src.resume()
            signalSources.append(src)
        }
    }

    // MARK: - Status bar

    private func setupStatusBar() {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        updateStatusIcon()
        rebuildMenu()
    }

    private func updateStatusIcon() {
        guard let btn = statusItem?.button else { return }
        btn.imagePosition = .imageLeading

        func setSymbol(_ name: String, label: String, tooltip: String) {
            let img = NSImage(systemSymbolName: name, accessibilityDescription: "SpellcheckMac")
            img?.isTemplate = true
            btn.image = img
            btn.title = label
            btn.toolTip = tooltip
        }

        if !AXIsProcessTrusted() || !monitor.isTapRunning {
            setSymbol("exclamationmark.triangle", label: "Ая", tooltip: "SpellcheckMac: нужен доступ Accessibility")
            return
        }
        if !config.wordAutocorrectEnabled && !config.sentenceCheckEnabled {
            setSymbol("pause.circle", label: "Ая", tooltip: "SpellcheckMac: на паузе")
            return
        }
        switch serverState {
        case .starting:
            setSymbol("ellipsis.circle", label: "Ая", tooltip: "SpellcheckMac: загрузка модели предложений…")
        case .ready(let model):
            setSymbol("text.badge.checkmark", label: "Ая", tooltip: "SpellcheckMac: активно (\(model))")
        case .offline(let reason):
            setSymbol("a.circle", label: "Ая", tooltip: "SpellcheckMac: только слова (\(reason))")
        }
    }

    private func rebuildMenu() {
        let menu = NSMenu()

        let statusLine: String
        if !AXIsProcessTrusted() || !monitor.isTapRunning {
            statusLine = "⚠️ Требуется доступ в Accessibility"
        } else {
            switch serverState {
            case .starting: statusLine = "⏳ Модель предложений загружается…"
            case .ready(let m): statusLine = "✅ Активно · \(m)"
            case .offline(let r): statusLine = "⚠️ Сервер предложений оффлайн (\(r))"
            }
        }
        let headerItem = NSMenuItem(title: statusLine, action: nil, keyEquivalent: "")
        headerItem.isEnabled = false
        menu.addItem(headerItem)

        if !AXIsProcessTrusted() || !monitor.isTapRunning {
            let permItem = NSMenuItem(
                title: "Открыть настройки Accessibility…",
                action: #selector(openAccessibilitySettings),
                keyEquivalent: ""
            )
            permItem.target = self
            menu.addItem(permItem)
        }

        menu.addItem(.separator())

        let wordItem = NSMenuItem(
            title: "Автозамена опечаток в словах",
            action: #selector(toggleWordAutocorrect),
            keyEquivalent: ""
        )
        wordItem.state = config.wordAutocorrectEnabled ? .on : .off
        wordItem.target = self
        menu.addItem(wordItem)

        let sentItem = NSMenuItem(
            title: "Проверка предложений (поповер)",
            action: #selector(toggleSentenceCheck),
            keyEquivalent: ""
        )
        sentItem.state = config.sentenceCheckEnabled ? .on : .off
        sentItem.target = self
        menu.addItem(sentItem)

        let loginItem = NSMenuItem(
            title: "Автозапуск при входе",
            action: #selector(toggleLaunchAtLogin),
            keyEquivalent: ""
        )
        loginItem.state = LaunchAgentManager.isEnabled ? .on : .off
        loginItem.target = self
        menu.addItem(loginItem)

        if let bid = lastNonSelfBundleID, let name = lastNonSelfAppName {
            menu.addItem(.separator())
            let excluded = config.isExcluded(bundleID: bid)
            let appToggle = NSMenuItem(
                title: excluded ? "Включить проверку в «\(name)»" : "Исключить «\(name)»",
                action: #selector(toggleCurrentAppExclusion),
                keyEquivalent: ""
            )
            appToggle.target = self
            menu.addItem(appToggle)
        }

        menu.addItem(.separator())

        let hintItem = NSMenuItem(
            title: "Backspace — отмена автозамены · ⌥↩ — применить фразу",
            action: nil,
            keyEquivalent: ""
        )
        hintItem.isEnabled = false
        menu.addItem(hintItem)

        menu.addItem(.separator())

        let quitItem = NSMenuItem(title: "Выход", action: #selector(quitApp), keyEquivalent: "q")
        quitItem.target = self
        menu.addItem(quitItem)

        statusItem.menu = menu
    }

    // MARK: - Startup & permissions

    private func observeFrontmostApp() {
        NSWorkspace.shared.notificationCenter.addObserver(
            forName: NSWorkspace.didActivateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self,
                      let app = self.securityGuard.frontmostApp(),
                      app.processIdentifier != ProcessInfo.processInfo.processIdentifier
                else { return }
                self.lastNonSelfBundleID = app.bundleIdentifier
                self.lastNonSelfAppName = app.localizedName ?? app.bundleIdentifier
                self.rebuildMenu()
            }
        }
    }

    private func startServer() {
        guard config.sentenceCheckEnabled else {
            serverState = .offline("выключено")
            updateStatusIcon()
            rebuildMenu()
            return
        }
        sentenceClient.ensureServerRunning { [weak self] state in
            guard let self else { return }
            self.serverState = state
            self.updateStatusIcon()
            self.rebuildMenu()
        }
    }

    private func startMonitoringOrRequestPermission() {
        let promptKey = "AXTrustedCheckOptionPrompt"
        let opts = [promptKey: true] as CFDictionary
        let trusted = AXIsProcessTrustedWithOptions(opts)

        if trusted && monitor.start() {
            updateStatusIcon()
            rebuildMenu()
            return
        }

        updateStatusIcon()
        rebuildMenu()

        permissionPollTask?.cancel()
        permissionPollTask = Task { @MainActor [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(2))
                guard let self else { return }
                if AXIsProcessTrusted() && self.monitor.start() {
                    self.updateStatusIcon()
                    self.rebuildMenu()
                    self.popover.showToast("Доступ получен — проверка активна", anchorRect: nil)
                    return
                }
            }
        }
    }

    // MARK: - Actions

    @objc private func toggleWordAutocorrect() {
        config.wordAutocorrectEnabled.toggle()
        ConfigStore.save(config)
        updateStatusIcon()
        rebuildMenu()
    }

    @objc private func toggleSentenceCheck() {
        config.sentenceCheckEnabled.toggle()
        ConfigStore.save(config)
        if config.sentenceCheckEnabled {
            sentenceClient.setAutoRecoverEnabled(true)
            startServer()
        } else {
            popover.hide()
            sentenceClient.setAutoRecoverEnabled(false)
            serverState = .offline("выключено")
        }
        updateStatusIcon()
        rebuildMenu()
    }

    @objc private func toggleLaunchAtLogin() {
        let next = !LaunchAgentManager.isEnabled
        try? LaunchAgentManager.setEnabled(next)
        rebuildMenu()
    }

    @objc private func toggleCurrentAppExclusion() {
        guard let bid = lastNonSelfBundleID else { return }
        _ = config.toggleExclusion(bundleID: bid)
        ConfigStore.save(config)
        monitor.resetState(dismissPopover: true)
        rebuildMenu()
    }

    @objc private func openAccessibilitySettings() {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility") {
            NSWorkspace.shared.open(url)
        }
    }

    @objc private func quitApp() {
        NSApp.terminate(nil)
    }
}
