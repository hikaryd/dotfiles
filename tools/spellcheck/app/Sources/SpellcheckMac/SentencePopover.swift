import AppKit
import SpellcheckCore

@MainActor
final class SentencePopoverController {
    struct Suggestion {
        let original: String
        let trailingWhitespace: String
        let corrected: String
        let latencyMs: Double
        let canReplaceInPlace: Bool

        var originalFullSuffix: String { original + trailingWhitespace }
        var correctedWithTrailing: String { corrected + trailingWhitespace }
    }

    private let panel: NSPanel
    private let visualEffect: NSVisualEffectView
    private let titleLabel = NSTextField(labelWithString: "")
    private let latencyLabel = NSTextField(labelWithString: "")
    private let diffLabel = NSTextField(wrappingLabelWithString: "")
    private let applyButton = NSButton(title: "Исправить (⌥↩)", target: nil, action: nil)
    private let copyButton = NSButton(title: "Копировать", target: nil, action: nil)
    private let dismissButton = NSButton(title: "✕", target: nil, action: nil)
    private let buttonRow = NSStackView()

    private var currentSuggestion: Suggestion?
    private var toastHideTask: Task<Void, Never>?

    var onApply: ((Suggestion, UInt64?) -> Void)?
    var onApplyAborted: ((UInt64?) -> Void)?
    var onDismiss: (() -> Void)?

    var isVisible: Bool { panel.isVisible && currentSuggestion != nil }
    var panelFrame: NSRect { panel.frame }

    init() {
        let p = NSPanel(
            contentRect: NSRect(x: 0, y: 0, width: 420, height: 112),
            styleMask: [.nonactivatingPanel, .borderless],
            backing: .buffered,
            defer: false
        )
        p.level = .floating
        p.isOpaque = false
        p.backgroundColor = .clear
        p.hasShadow = true
        p.hidesOnDeactivate = false
        p.isMovableByWindowBackground = true
        p.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .transient]
        p.becomesKeyOnlyIfNeeded = true
        self.panel = p

        let vfx = NSVisualEffectView(frame: p.contentView!.bounds)
        vfx.autoresizingMask = [.width, .height]
        vfx.material = .hudWindow
        vfx.blendingMode = .behindWindow
        vfx.state = .active
        vfx.wantsLayer = true
        vfx.layer?.cornerRadius = 12
        vfx.layer?.masksToBounds = true
        vfx.layer?.borderWidth = 1
        vfx.layer?.borderColor = NSColor.white.withAlphaComponent(0.14).cgColor
        self.visualEffect = vfx

        p.contentView?.addSubview(vfx)
        setupLayout()
    }

    private func setupLayout() {
        titleLabel.font = .systemFont(ofSize: 11, weight: .semibold)
        titleLabel.textColor = .secondaryLabelColor

        latencyLabel.font = .monospacedDigitSystemFont(ofSize: 10, weight: .regular)
        latencyLabel.textColor = .tertiaryLabelColor

        dismissButton.bezelStyle = .inline
        dismissButton.isBordered = false
        dismissButton.font = .systemFont(ofSize: 12, weight: .medium)
        dismissButton.contentTintColor = .secondaryLabelColor
        dismissButton.target = self
        dismissButton.action = #selector(handleDismissClick)

        let header = NSStackView(views: [titleLabel, latencyLabel, NSView(), dismissButton])
        header.orientation = .horizontal
        header.alignment = .centerY
        header.spacing = 6

        diffLabel.maximumNumberOfLines = 4
        diffLabel.lineBreakMode = .byWordWrapping
        diffLabel.isSelectable = false
        diffLabel.preferredMaxLayoutWidth = 392

        applyButton.bezelStyle = .rounded
        applyButton.controlSize = .small
        applyButton.keyEquivalent = ""
        applyButton.contentTintColor = .controlAccentColor
        applyButton.target = self
        applyButton.action = #selector(handleApplyClick)

        copyButton.bezelStyle = .rounded
        copyButton.controlSize = .small
        copyButton.target = self
        copyButton.action = #selector(handleCopyClick)

        buttonRow.orientation = .horizontal
        buttonRow.alignment = .centerY
        buttonRow.spacing = 8
        buttonRow.addArrangedSubview(applyButton)
        buttonRow.addArrangedSubview(copyButton)
        buttonRow.addArrangedSubview(NSView())

        let root = NSStackView(views: [header, diffLabel, buttonRow])
        root.orientation = .vertical
        root.alignment = .leading
        root.spacing = 8
        root.translatesAutoresizingMaskIntoConstraints = false
        header.translatesAutoresizingMaskIntoConstraints = false
        buttonRow.translatesAutoresizingMaskIntoConstraints = false

        visualEffect.addSubview(root)
        NSLayoutConstraint.activate([
            root.topAnchor.constraint(equalTo: visualEffect.topAnchor, constant: 10),
            root.leadingAnchor.constraint(equalTo: visualEffect.leadingAnchor, constant: 14),
            root.trailingAnchor.constraint(equalTo: visualEffect.trailingAnchor, constant: -14),
            root.bottomAnchor.constraint(equalTo: visualEffect.bottomAnchor, constant: -10),
            header.widthAnchor.constraint(equalTo: root.widthAnchor),
            buttonRow.widthAnchor.constraint(equalTo: root.widthAnchor),
        ])
    }

    func showSuggestion(_ suggestion: Suggestion, anchorRect: NSRect?) {
        toastHideTask?.cancel()
        toastHideTask = nil
        currentSuggestion = suggestion

        titleLabel.stringValue = "Предложение"
        latencyLabel.stringValue = String(format: "%.0f мс", suggestion.latencyMs)
        diffLabel.attributedStringValue = buildAttributedDiff(
            original: suggestion.original,
            corrected: suggestion.corrected
        )
        applyButton.isHidden = !suggestion.canReplaceInPlace
        buttonRow.isHidden = false
        dismissButton.isHidden = false

        layoutAndPosition(anchorRect: anchorRect, width: 420)
        panel.orderFrontRegardless()
    }

    func showToast(_ message: String, anchorRect: NSRect?) {
        toastHideTask?.cancel()
        // If a sentence suggestion was open, notify dismissal so TapSharedState never stays stuck on popoverVisible.
        if currentSuggestion != nil {
            currentSuggestion = nil
            onDismiss?()
        }

        titleLabel.stringValue = "Автозамена"
        latencyLabel.stringValue = ""
        diffLabel.attributedStringValue = NSAttributedString(
            string: message,
            attributes: [
                .font: NSFont.systemFont(ofSize: 12.5, weight: .medium),
                .foregroundColor: NSColor.labelColor,
            ]
        )
        buttonRow.isHidden = true
        dismissButton.isHidden = true

        layoutAndPosition(anchorRect: anchorRect, width: 260)
        panel.orderFrontRegardless()

        toastHideTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(1400))
            guard !Task.isCancelled else { return }
            self?.panel.orderOut(nil)
        }
    }

    func updateTrailingWhitespace(_ trailingWhitespace: String) {
        guard let cur = currentSuggestion else { return }
        currentSuggestion = Suggestion(
            original: cur.original,
            trailingWhitespace: trailingWhitespace,
            corrected: cur.corrected,
            latencyMs: cur.latencyMs,
            canReplaceInPlace: cur.canReplaceInPlace
        )
    }

    func triggerApplyCurrent(preArmedGeneration: UInt64? = nil) {
        guard let suggestion = currentSuggestion else {
            onApplyAborted?(preArmedGeneration)
            return
        }
        hide()
        onApply?(suggestion, preArmedGeneration)
    }

    func hide() {
        toastHideTask?.cancel()
        toastHideTask = nil
        let hadSuggestion = currentSuggestion != nil
        currentSuggestion = nil
        panel.orderOut(nil)
        if hadSuggestion {
            onDismiss?()
        }
    }

    @objc private func handleApplyClick() {
        triggerApplyCurrent()
    }

    @objc private func handleCopyClick() {
        guard let suggestion = currentSuggestion else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(suggestion.corrected, forType: .string)
        hide()
        showToast("Скопировано в буфер", anchorRect: nil)
    }

    @objc private func handleDismissClick() {
        hide()
    }

    private func layoutAndPosition(anchorRect: NSRect?, width: CGFloat) {
        diffLabel.preferredMaxLayoutWidth = width - 28
        panel.setContentSize(NSSize(width: width, height: 10))
        visualEffect.layoutSubtreeIfNeeded()
        let fitting = visualEffect.fittingSize
        let height = max(56, min(220, fitting.height))
        let size = NSSize(width: width, height: height)

        let screen = screenContaining(anchorRect) ?? NSScreen.main ?? NSScreen.screens.first
        let visible = screen?.visibleFrame ?? NSRect(x: 0, y: 0, width: 1440, height: 900)

        var origin: NSPoint
        if let anchor = anchorRect {
            var x = anchor.origin.x
            var y = anchor.maxY + 10
            if y + size.height > visible.maxY - 8 {
                y = anchor.minY - size.height - 10
            }
            x = max(visible.minX + 12, min(x, visible.maxX - size.width - 12))
            y = max(visible.minY + 12, min(y, visible.maxY - size.height - 12))
            origin = NSPoint(x: x, y: y)
        } else {
            origin = NSPoint(
                x: visible.maxX - size.width - 20,
                y: visible.maxY - size.height - 16
            )
        }

        panel.setFrame(NSRect(origin: origin, size: size), display: true)
    }

    private func screenContaining(_ rect: NSRect?) -> NSScreen? {
        guard let rect else { return nil }
        return NSScreen.screens.first { $0.frame.intersects(rect) }
    }

    private func buildAttributedDiff(original: String, corrected: String) -> NSAttributedString {
        let segments = DiffBuilder.diff(original: original, corrected: corrected)
        let out = NSMutableAttributedString()
        let baseFont = NSFont.systemFont(ofSize: 13.5, weight: .regular)
        let boldFont = NSFont.systemFont(ofSize: 13.5, weight: .semibold)

        for seg in segments {
            switch seg.kind {
            case .equal:
                out.append(NSAttributedString(string: seg.text, attributes: [
                    .font: baseFont,
                    .foregroundColor: NSColor.labelColor,
                ]))
            case .deleted:
                out.append(NSAttributedString(string: seg.text, attributes: [
                    .font: baseFont,
                    .foregroundColor: NSColor.systemRed.withAlphaComponent(0.9),
                    .strikethroughStyle: NSUnderlineStyle.single.rawValue,
                ]))
            case .inserted:
                out.append(NSAttributedString(string: seg.text, attributes: [
                    .font: boldFont,
                    .foregroundColor: NSColor.systemGreen,
                ]))
            }
        }
        return out
    }
}
