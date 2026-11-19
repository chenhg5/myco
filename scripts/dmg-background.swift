// Draws the DMG window's background: one line saying what to do, and an
// arrow from where myco sits to where Applications sits.
//
//   swift scripts/dmg-background.swift cmd/myco-app/build/darwin
//
// writes dmg-background.png (640x400) and dmg-background@2x.png. The window,
// its icon positions and this picture are set together in
// scripts/mac-sign.sh: change them together.
import AppKit

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "."
let size = NSSize(width: 640, height: 400)
// Where the two icons' centres are (from the top left, as Finder counts).
let app = NSPoint(x: 170, y: 190), apps = NSPoint(x: 470, y: 190)

func rgb(_ hex: UInt32, _ alpha: CGFloat = 1) -> NSColor {
    NSColor(srgbRed: CGFloat((hex >> 16) & 0xFF) / 255, green: CGFloat((hex >> 8) & 0xFF) / 255, blue: CGFloat(hex & 0xFF) / 255, alpha: alpha)
}

func draw(scale: CGFloat) -> Data {
    let pixels = NSSize(width: size.width * scale, height: size.height * scale)
    let rep = NSBitmapImageRep(
        bitmapDataPlanes: nil, pixelsWide: Int(pixels.width), pixelsHigh: Int(pixels.height), bitsPerSample: 8,
        samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    rep.size = size
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    // (AppKit counts from the bottom; Finder, and the numbers here, from the top)
    func up(_ y: CGFloat) -> CGFloat { size.height - y }

    // The app's light surface, a little lighter at the top. Light on
    // purpose: Finder draws the icons' names dark on a window with a
    // picture, in dark mode too.
    NSGradient(starting: rgb(0xF1F1F4), ending: rgb(0xFCFCFD))!.draw(in: NSRect(origin: .zero, size: size), angle: 90)

    // What to do, at the top: Chinese, then English, quiet.
    let center = NSMutableParagraphStyle()
    center.alignment = .center
    func line(_ text: String, y: CGFloat, font: NSFont, color: NSColor) {
        let attributes: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: color, .paragraphStyle: center]
        let height = (text as NSString).size(withAttributes: attributes).height
        (text as NSString).draw(in: NSRect(x: 0, y: up(y) - height, width: size.width, height: height), withAttributes: attributes)
    }
    let zh = NSFont(name: "PingFangSC-Medium", size: 17) ?? .systemFont(ofSize: 17, weight: .medium)
    line("把 myco 拖到「应用程序」文件夹", y: 40, font: zh, color: rgb(0x1C1C21))
    line("Drag myco to Applications to install", y: 66, font: .systemFont(ofSize: 12.5), color: rgb(0x85858D))

    // The arrow between the icons: a soft line and a rounded head.
    let start = app.x + 78, end = apps.x - 78, y = up(app.y)
    let shaft = NSBezierPath()
    shaft.move(to: NSPoint(x: start, y: y))
    shaft.line(to: NSPoint(x: end - 4, y: y))
    shaft.lineWidth = 3
    shaft.lineCapStyle = .round
    rgb(0xB9B9C2).setStroke()
    shaft.stroke()
    let head = NSBezierPath()
    head.move(to: NSPoint(x: end - 12, y: y - 10))
    head.line(to: NSPoint(x: end, y: y))
    head.line(to: NSPoint(x: end - 12, y: y + 10))
    head.lineWidth = 3
    head.lineCapStyle = .round
    head.lineJoinStyle = .round
    head.stroke()

    // A faint mushroom-green line at the bottom: the brand, no more.
    line("myco", y: 352, font: .systemFont(ofSize: 11, weight: .semibold), color: rgb(0x3A9A78, 0.55))

    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

try draw(scale: 1).write(to: URL(fileURLWithPath: out + "/dmg-background.png"))
try draw(scale: 2).write(to: URL(fileURLWithPath: out + "/dmg-background@2x.png"))
print("wrote \(out)/dmg-background.png and dmg-background@2x.png")
