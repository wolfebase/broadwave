import UIKit

@MainActor
enum ScreenIdentity {
    static var name: String {
        UIDevice.current.name
    }

    static var kind: String {
        #if os(tvOS)
            "appletv"
        #else
            UIDevice.current.userInterfaceIdiom == .pad ? "ipad" : "iphone"
        #endif
    }
}
