import AVFoundation
import BroadwaveKit
import MediaAccessibility
import SwiftUI

/// Settings › Captions: the device's caption styles with a live preview over a still
/// from the last channel, and whether captions always show. Both are the system's
/// own settings, so every app and the system player follow them.
struct CaptionStyleSection: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    @State private var profiles: [CaptionProfile] = []
    @State private var active = ""
    @State private var always = false

    var body: some View {
        Section {
            if #available(iOS 26.4, tvOS 26.4, *), !active.isEmpty {
                CaptionPreview(profileID: active, still: still)
                    .aspectRatio(16 / 9, contentMode: .fit)
                    .clipShape(.rect(cornerRadius: 14))
                    .listRowInsets(EdgeInsets(top: 8, leading: 8, bottom: 8, trailing: 8))
                    .accessibilityElement()
                    .accessibilityLabel("Caption preview, \(profiles.first { $0.id == active }?.name ?? "")")
                    .accessibilityIdentifier("caption-preview")
            }
            Toggle("Always show captions", isOn: Binding(get: { always }, set: { on in
                always = on
                MACaptionAppearanceSetDisplayType(.user, on ? .alwaysOn : .automatic)
            }))
            .accessibilityIdentifier("caption-always")
            if profiles.count > 1 {
                Picker("Style", selection: Binding(get: { active }, set: { id in
                    active = id
                    MACaptionAppearanceSetActiveProfileID(id as CFString)
                })) {
                    ForEach(profiles) { profile in
                        Text(profile.name).tag(profile.id)
                    }
                }
                .accessibilityIdentifier("caption-style")
            }
        } header: {
            Text("Captions")
        } footer: {
            Text("The style is this device's, so other apps use it too. A channel that sends no captions can still have them on iOS 27 and tvOS 27, which make them as it plays.")
        }
        .onAppear(perform: load)
    }

    private func load() {
        profiles = CaptionProfile.all()
        active = MACaptionAppearanceCopyActiveProfileID() as String
        always = MACaptionAppearanceGetDisplayType(.user) == .alwaysOn
    }

    /// A real picture under the preview: a still from the channel watched last, else from any
    /// channel on now, else the art of a program on now.
    private var still: URL? {
        let last = nowPlaying.lastChannel?.id ?? NowPlaying.lastChannelID
        let framed = store.frameIDs.contains(last) ? last : store.channels.first { store.frameIDs.contains($0.id) }?.id
        if let framed {
            return store.api?.frameURL(channelID: framed, width: 1280)
        }
        return store.channels.lazy.compactMap { store.index.on($0.id, at: store.now) }.compactMap { store.artURL($0, width: 1280) }.first
    }
}

struct CaptionProfile: Identifiable, Hashable {
    let id: String
    let name: String

    static func all() -> [CaptionProfile] {
        let ids = MACaptionAppearanceCopyProfileIDs() as? [String] ?? []
        return ids.map { id in
            CaptionProfile(id: id, name: MACaptionAppearanceCopyProfileName(id as CFString) as String)
        }
    }
}

/// The system's own caption preview, drawn by an AVPlayerLayer over the still.
@available(iOS 26.4, tvOS 26.4, *)
private struct CaptionPreview: View {
    let profileID: String
    let still: URL?

    var body: some View {
        ZStack {
            LinearGradient(colors: [Color(white: 0.22), Color(white: 0.06)], startPoint: .top, endPoint: .bottom)
            if let still {
                AsyncImage(url: still) { image in
                    image.resizable().scaledToFill()
                } placeholder: {
                    Color.clear
                }
                .transition(.opacity)
            }
            CaptionLayer(profileID: profileID)
        }
    }
}

@available(iOS 26.4, tvOS 26.4, *)
private struct CaptionLayer: UIViewRepresentable {
    let profileID: String

    func makeUIView(context _: Context) -> CaptionLayerView {
        let view = CaptionLayerView()
        view.isUserInteractionEnabled = false
        view.backgroundColor = .clear
        return view
    }

    func updateUIView(_ view: CaptionLayerView, context _: Context) {
        view.show(profileID)
    }

    static func dismantleUIView(_ view: CaptionLayerView, coordinator _: ()) {
        view.playerLayer.stopShowingCaptionPreview()
    }
}

@available(iOS 26.4, tvOS 26.4, *)
private final class CaptionLayerView: UIView {
    override static var layerClass: AnyClass {
        AVPlayerLayer.self
    }

    var playerLayer: AVPlayerLayer {
        // swiftlint:disable:next force_cast
        layer as! AVPlayerLayer
    }

    private var shown = ""

    func show(_ profileID: String) {
        guard profileID != shown else { return }
        shown = profileID
        if playerLayer.player == nil {
            playerLayer.player = AVPlayer()
        }
        playerLayer.setCaptionPreviewProfileID(profileID, position: .zero, text: String(localized: "Captions look like this."))
    }
}
