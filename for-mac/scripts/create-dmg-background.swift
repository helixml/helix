#!/usr/bin/env swift
// Generate a DMG background image for Helix for Mac.
//
// Creates a 660x400 background with:
// - Dark gradient background using colors from the Helix logo palette
// - Subtle right-pointing arrow between icon positions
//
// Uses only macOS built-in frameworks (CoreGraphics).
//
// Usage:
//   swift scripts/create-dmg-background.swift
//   swift scripts/create-dmg-background.swift --output /path/to/output.png

import Foundation
import CoreGraphics
import ImageIO

// MARK: - Constants

let WIDTH = 660
let HEIGHT = 400

// Icon positions (where Finder places the .app and Applications icons)
let APP_ICON_X: CGFloat = 170
let APPLICATIONS_X: CGFloat = 490
let ICON_Y: CGFloat = 175  // vertical center for icons (Finder coordinates, top-down)

// Colors — white background with subtle gray chevron arrow (like Google Drive DMG)
let BG_TOP = (r: 1.0, g: 1.0, b: 1.0)                                   // #FFFFFF white
let BG_BOTTOM = (r: 1.0, g: 1.0, b: 1.0)                                // #FFFFFF white
let ARROW_COLOR = (r: 0.72, g: 0.72, b: 0.72)                           // Light gray #B8B8B8
let ARROW_ALPHA = 1.0                                                     // Fully opaque

// MARK: - Argument parsing

var outputPath: String? = nil

var args = CommandLine.arguments.dropFirst()
while let arg = args.first {
    args = args.dropFirst()
    switch arg {
    case "--output":
        outputPath = args.first
        args = args.dropFirst()
    default:
        fputs("Unknown option: \(arg)\n", stderr)
        exit(1)
    }
}

// Determine paths
let scriptDir = URL(fileURLWithPath: #filePath).deletingLastPathComponent().path
let forMacDir = URL(fileURLWithPath: scriptDir).deletingLastPathComponent().path
let assetsDir = "\(forMacDir)/assets"

let finalOutputPath = outputPath ?? "\(assetsDir)/dmg-background.png"

// Create assets directory
try? FileManager.default.createDirectory(atPath: assetsDir, withIntermediateDirectories: true)

// MARK: - Create bitmap context

let colorSpace = CGColorSpaceCreateDeviceRGB()
guard let ctx = CGContext(
    data: nil,
    width: WIDTH,
    height: HEIGHT,
    bitsPerComponent: 8,
    bytesPerRow: WIDTH * 4,
    space: colorSpace,
    bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
) else {
    fputs("ERROR: Could not create bitmap context\n", stderr)
    exit(1)
}

ctx.setAllowsAntialiasing(true)
ctx.setShouldAntialias(true)

// MARK: - Draw gradient background

for i in 0..<HEIGHT {
    let t = CGFloat(i) / CGFloat(HEIGHT - 1)
    // CG origin is bottom-left: row 0 = bottom = BG_BOTTOM, row HEIGHT-1 = top = BG_TOP
    let r = CGFloat(BG_BOTTOM.r + (BG_TOP.r - BG_BOTTOM.r) * Double(t))
    let g = CGFloat(BG_BOTTOM.g + (BG_TOP.g - BG_BOTTOM.g) * Double(t))
    let b = CGFloat(BG_BOTTOM.b + (BG_TOP.b - BG_BOTTOM.b) * Double(t))
    ctx.setFillColor(red: r, green: g, blue: b, alpha: 1.0)
    ctx.fill(CGRect(x: 0, y: i, width: WIDTH, height: 1))
}

// MARK: - Draw arrow (three triangles >>> with fade, like a "drag here" indicator)

let arrowY = CGFloat(HEIGHT) - ICON_Y  // flip to CG coords
let centerX = (APP_ICON_X + APPLICATIONS_X) / 2  // center between icons

let triangleWidth: CGFloat = 14    // horizontal size of each triangle
let triangleHeight: CGFloat = 24   // vertical size of each triangle
let spacing: CGFloat = 22          // center-to-center distance between triangles
let count = 3

// Alphas fade from left (lighter) to right (darker) to suggest direction
let alphas: [CGFloat] = [0.35, 0.55, 0.80]

for i in 0..<count {
    let offset = CGFloat(i - count / 2) * spacing
    let tipX = centerX + offset + triangleWidth / 2
    let baseX = centerX + offset - triangleWidth / 2

    ctx.saveGState()
    ctx.setFillColor(red: CGFloat(ARROW_COLOR.r), green: CGFloat(ARROW_COLOR.g), blue: CGFloat(ARROW_COLOR.b), alpha: alphas[i])

    ctx.beginPath()
    ctx.move(to: CGPoint(x: tipX, y: arrowY))
    ctx.addLine(to: CGPoint(x: baseX, y: arrowY + triangleHeight / 2))
    ctx.addLine(to: CGPoint(x: baseX, y: arrowY - triangleHeight / 2))
    ctx.closePath()
    ctx.fillPath()

    ctx.restoreGState()
}

// MARK: - Save PNG

guard let image = ctx.makeImage() else {
    fputs("ERROR: Could not create image from context\n", stderr)
    exit(1)
}

let outputURL = URL(fileURLWithPath: finalOutputPath) as CFURL
guard let dest = CGImageDestinationCreateWithURL(outputURL, "public.png" as CFString, 1, nil) else {
    fputs("ERROR: Could not create image destination at \(finalOutputPath)\n", stderr)
    exit(1)
}
CGImageDestinationAddImage(dest, image, nil)
CGImageDestinationFinalize(dest)

let fileSize = try! FileManager.default.attributesOfItem(atPath: finalOutputPath)[.size] as! Int
print("Saved: \(finalOutputPath)")
print("Size: \(fileSize) bytes (\(fileSize / 1024) KB)")
