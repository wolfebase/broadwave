import BroadwaveKit
import UIKit

@MainActor
enum ScreenIdentity {
    /// So another screen can send this one a channel. The vendor id, which a restore
    /// from another device's backup does not copy, or a kept one when there is none.
    static var id: String {
        ScreenID.current(vendor: UIDevice.current.identifierForVendor?.uuidString)
    }

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
