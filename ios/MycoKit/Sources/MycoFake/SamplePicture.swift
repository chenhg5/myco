import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

/// Pictures drawn here, so the demo workspace has some to show without
/// shipping any: soft colour, a few shapes, like a sketch on a board.
public enum SamplePicture {
    public static func png(hue: Double, width: Int, height: Int) -> Data {
        let space = CGColorSpaceCreateDeviceRGB()
        guard let context = CGContext(
            data: nil, width: width, height: height, bitsPerComponent: 8, bytesPerRow: 0, space: space,
            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
        else { return Data() }
        let w = CGFloat(width), h = CGFloat(height)
        let top = color(hue, 0.22, 0.97), bottom = color(hue + 0.06, 0.34, 0.86)
        if let gradient = CGGradient(colorsSpace: space, colors: [bottom, top] as CFArray, locations: [0, 1]) {
            context.drawLinearGradient(gradient, start: .zero, end: CGPoint(x: w * 0.3, y: h), options: [])
        }
        // A phone's outline with three rows and a bar: a page being sketched.
        let frame = CGRect(x: w * 0.3, y: h * 0.14, width: w * 0.4, height: h * 0.72)
        context.setStrokeColor(color(hue, 0.5, 0.42))
        context.setLineWidth(max(3, w / 220))
        context.addPath(CGPath(roundedRect: frame, cornerWidth: w * 0.035, cornerHeight: w * 0.035, transform: nil))
        context.strokePath()
        context.setFillColor(color(hue, 0.42, 0.58, alpha: 0.55))
        for row in 0..<3 {
            let y = frame.maxY - frame.height * (0.2 + 0.17 * CGFloat(row))
            context.fillEllipse(in: CGRect(x: frame.minX + frame.width * 0.1, y: y, width: frame.width * 0.14, height: frame.width * 0.14))
            context.fill(CGRect(x: frame.minX + frame.width * 0.32, y: y + frame.width * 0.04, width: frame.width * 0.5, height: frame.width * 0.06))
        }
        context.fill(CGRect(x: frame.minX + frame.width * 0.1, y: frame.minY + frame.height * 0.06, width: frame.width * 0.8, height: frame.height * 0.07))
        guard let image = context.makeImage() else { return Data() }
        let out = NSMutableData()
        guard let destination = CGImageDestinationCreateWithData(out, UTType.png.identifier as CFString, 1, nil) else { return Data() }
        CGImageDestinationAddImage(destination, image, nil)
        return CGImageDestinationFinalize(destination) ? out as Data : Data()
    }

    private static func color(_ hue: Double, _ saturation: Double, _ brightness: Double, alpha: Double = 1) -> CGColor {
        // HSB to RGB, by hand: no UIKit here.
        let h = (hue - hue.rounded(.down)) * 6
        let c = brightness * saturation
        let x = c * (1 - abs(h.truncatingRemainder(dividingBy: 2) - 1))
        let m = brightness - c
        let rgb: (r: Double, g: Double, b: Double)
        switch Int(h) {
        case 0: rgb = (c, x, 0)
        case 1: rgb = (x, c, 0)
        case 2: rgb = (0, c, x)
        case 3: rgb = (0, x, c)
        case 4: rgb = (x, 0, c)
        default: rgb = (c, 0, x)
        }
        let (r, g, b) = rgb
        return CGColor(colorSpace: CGColorSpaceCreateDeviceRGB(), components: [CGFloat(r + m), CGFloat(g + m), CGFloat(b + m), CGFloat(alpha)]) ?? CGColor(gray: 0.5, alpha: 1)
    }
}
