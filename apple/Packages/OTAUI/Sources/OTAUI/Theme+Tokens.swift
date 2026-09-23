// Generated from design/tokens.json by design/build.mjs. Do not edit.

import SwiftUI

public enum Tokens {
    public enum ColorToken {
        public static let canvas = Color(.sRGB, red: 0.027, green: 0.039, blue: 0.063, opacity: 1.000)
        public static let surface1 = Color(.sRGB, red: 0.055, green: 0.075, blue: 0.110, opacity: 1.000)
        public static let surface2 = Color(.sRGB, red: 0.082, green: 0.110, blue: 0.157, opacity: 1.000)
        public static let surface3 = Color(.sRGB, red: 0.118, green: 0.153, blue: 0.212, opacity: 1.000)
        public static let line = Color(.sRGB, red: 1.000, green: 1.000, blue: 1.000, opacity: 0.102)
        public static let lineStrong = Color(.sRGB, red: 1.000, green: 1.000, blue: 1.000, opacity: 0.200)
        public static let text = Color(.sRGB, red: 0.961, green: 0.969, blue: 0.980, opacity: 1.000)
        public static let textSecondary = Color(.sRGB, red: 0.682, green: 0.718, blue: 0.773, opacity: 1.000)
        public static let textTertiary = Color(.sRGB, red: 0.451, green: 0.494, blue: 0.561, opacity: 1.000)
        public static let accent = Color(.sRGB, red: 0.239, green: 0.482, blue: 1.000, opacity: 1.000)
        public static let accentSoft = Color(.sRGB, red: 0.239, green: 0.482, blue: 1.000, opacity: 0.161)
        public static let onAccent = Color(.sRGB, red: 1.000, green: 1.000, blue: 1.000, opacity: 1.000)
        public static let tally = Color(.sRGB, red: 1.000, green: 0.231, blue: 0.188, opacity: 1.000)
        public static let tallySoft = Color(.sRGB, red: 1.000, green: 0.231, blue: 0.188, opacity: 0.161)
        public static let success = Color(.sRGB, red: 0.196, green: 0.843, blue: 0.294, opacity: 1.000)
        public static let warning = Color(.sRGB, red: 1.000, green: 0.702, blue: 0.251, opacity: 1.000)
        public static let glassFill = Color(.sRGB, red: 0.102, green: 0.129, blue: 0.188, opacity: 0.722)
        public static let glassStroke = Color(.sRGB, red: 1.000, green: 1.000, blue: 1.000, opacity: 0.141)
        public static let glassHighlight = Color(.sRGB, red: 1.000, green: 1.000, blue: 1.000, opacity: 0.078)
        public static let scrim = Color(.sRGB, red: 0.000, green: 0.000, blue: 0.000, opacity: 0.651)
    }

    public enum Category {
        public static let sports = Color(.sRGB, red: 0.184, green: 0.749, blue: 0.443, opacity: 1.000)
        public static let news = Color(.sRGB, red: 0.239, green: 0.608, blue: 1.000, opacity: 1.000)
        public static let movies = Color(.sRGB, red: 0.702, green: 0.420, blue: 1.000, opacity: 1.000)
        public static let kids = Color(.sRGB, red: 1.000, green: 0.690, blue: 0.125, opacity: 1.000)
        public static let series = Color(.sRGB, red: 0.369, green: 0.482, blue: 0.659, opacity: 1.000)
        public static let other = Color(.sRGB, red: 0.353, green: 0.392, blue: 0.455, opacity: 1.000)
    }

    public enum Radius {
        public static let xs: CGFloat = 6
        public static let sm: CGFloat = 10
        public static let md: CGFloat = 14
        public static let lg: CGFloat = 20
        public static let xl: CGFloat = 28
        public static let pill: CGFloat = 999
    }

    public enum Space {
        public static let s1: CGFloat = 4
        public static let s2: CGFloat = 8
        public static let s3: CGFloat = 12
        public static let s4: CGFloat = 16
        public static let s5: CGFloat = 20
        public static let s6: CGFloat = 24
        public static let s8: CGFloat = 32
        public static let s10: CGFloat = 40
        public static let s12: CGFloat = 48
        public static let s16: CGFloat = 64
    }

    public enum Motion {
        public static let fast: Double = 0.14
        public static let base: Double = 0.24
        public static let slow: Double = 0.42
        public static let spring = Animation.spring(response: 0.38, dampingFraction: 0.82)
    }

    public enum TypeSize {
        public static let caption: CGFloat = 12
        public static let footnote: CGFloat = 13
        public static let body: CGFloat = 16
        public static let headline: CGFloat = 17
        public static let title3: CGFloat = 20
        public static let title2: CGFloat = 26
        public static let title1: CGFloat = 34
        public static let display: CGFloat = 56
        public static let channel: CGFloat = 22
    }

    public static let tvScale: CGFloat = 1.6
}
