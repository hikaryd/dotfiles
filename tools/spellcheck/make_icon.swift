#!/usr/bin/env swift
import AppKit
import CoreGraphics
import Foundation

guard CommandLine.arguments.count >= 2 else {
    fputs("Usage: make_icon.swift <output_dir.iconset>\n", stderr)
    exit(1)
}

let outDir = URL(fileURLWithPath: CommandLine.arguments[1])
try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)

func renderIcon(pixelSize: Int) -> NSBitmapImageRep {
    let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil,
        pixelsWide: pixelSize,
        pixelsHigh: pixelSize,
        bitsPerSample: 8,
        samplesPerPixel: 4,
        hasAlpha: true,
        isPlanar: false,
        colorSpaceName: .deviceRGB,
        bytesPerRow: 0,
        bitsPerPixel: 0
    )!
    rep.size = NSSize(width: pixelSize, height: pixelSize)

    NSGraphicsContext.saveGraphicsState()
    let ctx = NSGraphicsContext(bitmapImageRep: rep)!
    NSGraphicsContext.current = ctx
    let cg = ctx.cgContext

    let s = CGFloat(pixelSize)
    let scale = s / 1024.0

    cg.setAllowsAntialiasing(true)
    cg.setShouldAntialias(true)
    cg.interpolationQuality = .high

    // Apple HIG macOS squircle proportion (824x824 centered inside 1024x1024)
    let inset = 100.0 * scale
    let cardRect = CGRect(x: inset, y: inset, width: s - 2 * inset, height: s - 2 * inset)
    let cornerRadius = 186.0 * scale
    let cardPath = NSBezierPath(roundedRect: cardRect, xRadius: cornerRadius, yRadius: cornerRadius)

    // Subtle elevation shadow
    cg.saveGState()
    let shadow = NSShadow()
    shadow.shadowColor = NSColor.black.withAlphaComponent(0.32)
    shadow.shadowOffset = NSSize(width: 0, height: -18 * scale)
    shadow.shadowBlurRadius = 28 * scale
    shadow.set()
    NSColor(calibratedRed: 0.10, green: 0.13, blue: 0.20, alpha: 1.0).setFill()
    cardPath.fill()
    cg.restoreGState()

    // Rich slate-indigo vertical gradient clipped to squircle
    cg.saveGState()
    cardPath.addClip()
    let colorSpace = CGColorSpaceCreateDeviceRGB()
    let colors = [
        CGColor(red: 0.16, green: 0.21, blue: 0.32, alpha: 1.0),
        CGColor(red: 0.08, green: 0.11, blue: 0.17, alpha: 1.0),
    ] as CFArray
    if let grad = CGGradient(colorsSpace: colorSpace, colors: colors, locations: [0.0, 1.0]) {
        cg.drawLinearGradient(
            grad,
            start: CGPoint(x: s / 2, y: cardRect.maxY),
            end: CGPoint(x: s / 2, y: cardRect.minY),
            options: []
        )
    }

    // Subtle top specular highlight
    let highlightRect = CGRect(x: cardRect.minX, y: cardRect.midY, width: cardRect.width, height: cardRect.height / 2)
    let hiColors = [
        CGColor(red: 1, green: 1, blue: 1, alpha: 0.08),
        CGColor(red: 1, green: 1, blue: 1, alpha: 0.0),
    ] as CFArray
    if let hiGrad = CGGradient(colorsSpace: colorSpace, colors: hiColors, locations: [0.0, 1.0]) {
        cg.drawLinearGradient(
            hiGrad,
            start: CGPoint(x: s / 2, y: cardRect.maxY),
            end: CGPoint(x: s / 2, y: highlightRect.minY),
            options: []
        )
    }
    cg.restoreGState()

    // Crisp inner rim stroke
    let strokeRect = cardRect.insetBy(dx: 2.0 * scale, dy: 2.0 * scale)
    let strokePath = NSBezierPath(roundedRect: strokeRect, xRadius: cornerRadius - 2 * scale, yRadius: cornerRadius - 2 * scale)
    strokePath.lineWidth = max(1.0, 3.0 * scale)
    NSColor.white.withAlphaComponent(0.14).setStroke()
    strokePath.stroke()

    // Main typography "Ая"
    let fontSize = 415.0 * scale
    let font = NSFont.systemFont(ofSize: fontSize, weight: .bold)
    let text = NSString(string: "Ая")
    let attrs: [NSAttributedString.Key: Any] = [
        .font: font,
        .foregroundColor: NSColor(calibratedRed: 0.96, green: 0.97, blue: 0.99, alpha: 1.0),
        .kern: -10.0 * scale,
    ]
    let textSize = text.size(withAttributes: attrs)
    let textOrigin = NSPoint(
        x: (s - textSize.width) / 2.0 - 18.0 * scale,
        y: (s - textSize.height) / 2.0 + 14.0 * scale
    )
    text.draw(at: textOrigin, withAttributes: attrs)

    // Emerald checkmark badge in bottom-right of squircle
    let badgeDiameter = 236.0 * scale
    let badgeCenter = CGPoint(x: cardRect.maxX - 134.0 * scale, y: cardRect.minY + 134.0 * scale)
    let badgeRect = CGRect(
        x: badgeCenter.x - badgeDiameter / 2,
        y: badgeCenter.y - badgeDiameter / 2,
        width: badgeDiameter,
        height: badgeDiameter
    )

    // Dark ring cutout separating badge from typography
    let ringRect = badgeRect.insetBy(dx: -16.0 * scale, dy: -16.0 * scale)
    NSColor(calibratedRed: 0.08, green: 0.11, blue: 0.17, alpha: 1.0).setFill()
    NSBezierPath(ovalIn: ringRect).fill()

    // Emerald gradient circle
    cg.saveGState()
    let badgePath = NSBezierPath(ovalIn: badgeRect)
    badgePath.addClip()
    let badgeColors = [
        CGColor(red: 0.20, green: 0.84, blue: 0.48, alpha: 1.0),
        CGColor(red: 0.09, green: 0.65, blue: 0.34, alpha: 1.0),
    ] as CFArray
    if let bGrad = CGGradient(colorsSpace: colorSpace, colors: badgeColors, locations: [0.0, 1.0]) {
        cg.drawLinearGradient(
            bGrad,
            start: CGPoint(x: badgeCenter.x, y: badgeRect.maxY),
            end: CGPoint(x: badgeCenter.x, y: badgeRect.minY),
            options: []
        )
    }
    cg.restoreGState()

    // Crisp white checkmark
    let check = NSBezierPath()
    check.move(to: CGPoint(x: badgeCenter.x - 54.0 * scale, y: badgeCenter.y - 4.0 * scale))
    check.line(to: CGPoint(x: badgeCenter.x - 14.0 * scale, y: badgeCenter.y - 46.0 * scale))
    check.line(to: CGPoint(x: badgeCenter.x + 58.0 * scale, y: badgeCenter.y + 42.0 * scale))
    check.lineWidth = max(1.5, 28.0 * scale)
    check.lineCapStyle = .round
    check.lineJoinStyle = .round
    NSColor.white.setStroke()
    check.stroke()

    NSGraphicsContext.restoreGraphicsState()
    return rep
}

let entries: [(String, Int)] = [
    ("icon_16x16.png", 16),
    ("icon_16x16@2x.png", 32),
    ("icon_32x32.png", 32),
    ("icon_32x32@2x.png", 64),
    ("icon_128x128.png", 128),
    ("icon_128x128@2x.png", 256),
    ("icon_256x256.png", 256),
    ("icon_256x256@2x.png", 512),
    ("icon_512x512.png", 512),
    ("icon_512x512@2x.png", 1024),
]

for (name, px) in entries {
    let rep = renderIcon(pixelSize: px)
    guard let png = rep.representation(using: .png, properties: [:]) else {
        fputs("Failed to encode \(name)\n", stderr)
        exit(1)
    }
    try png.write(to: outDir.appendingPathComponent(name))
}
print("Generated iconset at \(outDir.path)")
