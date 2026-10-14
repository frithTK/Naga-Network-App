#if canImport(SwiftUI)
import SwiftUI
import UIKit

enum NagaTheme {
    static let background = Color(hex: 0x080808)
    static let surfaceLow = Color(hex: 0x111215)
    static let surface = Color(hex: 0x1A1C20)
    static let raised = Color(hex: 0x202227)
    static let line = Color(hex: 0x282E36)
    static let accent = Color(hex: 0xF72F38)
    static let success = Color(hex: 0x27C46A)
    static let warning = Color(hex: 0xFFB020)
    static let error = Color(hex: 0xFF454D)
    static let text = Color.white
    static let muted = Color(hex: 0xABB0B9)
    static let tertiary = Color(hex: 0x9299A5)

    static func font(_ size: CGFloat = 14, weight: Font.Weight = .regular,
                     relativeTo style: Font.TextStyle = .body) -> Font {
        let name: String
        switch weight {
        case .bold: name = "Inter-Bold"
        case .semibold: name = "Inter-SemiBold"
        case .medium: name = "Inter-Medium"
        default: name = "Inter-Regular"
        }
        return .custom(name, size: size, relativeTo: style)
    }
}

private extension Color {
    init(hex: UInt32) {
        self.init(red: Double((hex >> 16) & 255) / 255,
                  green: Double((hex >> 8) & 255) / 255, blue: Double(hex & 255) / 255)
    }
}

enum NagaAssets {
    static func image(_ name: String, directory: String) -> Image? {
        for folder in [directory, "Resources/\(directory)"] {
            if let path = Bundle.main.path(forResource: name, ofType: "png", inDirectory: folder),
               let image = UIImage(contentsOfFile: path) { return Image(uiImage: image) }
        }
        if let image = UIImage(named: name) { return Image(uiImage: image) }
        return nil
    }
}
#endif
