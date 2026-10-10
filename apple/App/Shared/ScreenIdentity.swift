import BroadwaveKit
import UIKit

@MainActor
enum ScreenIdentity {
    /// So another screen can send this one a channel. The vendor id, which a restore
    /// from another device's backup does not copy, or a kept one when there is none.
    static var id: String {
        ScreenID.current(vendor: UIDevice.current.identifierForVendor?.uuidString)
    }

    /// The name iOS and tvOS report. On 16 and later that is "iPhone" or "Apple TV".
    static var deviceName: String {
        UIDevice.current.name
    }

    /// A name from Settings, or the device name when none is saved.
    static var name: String {
        ScreenName.current(device: deviceName)
    }

    static var kind: String {
        #if os(tvOS)
            "appletv"
        #else
            UIDevice.current.userInterfaceIdiom == .pad ? "ipad" : "iphone"
        #endif
    }
}
