import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// Program art for one slot. Bleed fills; anything smaller stays near its real size.
/// Slot width is in pixels, so 1.25× is measured against the picture, not points.
struct ProgramPicture: View {
    let url: URL
    var width: Int
    var height: Int
    /// A thumbnail only crops or fits. A hero may sit a sharp picture on a blur.
    var hero = true

    @Environment(\.displayScale) private var displayScale

    var body: some View {
        GeometryReader { geo in
            let scale = Double(max(displayScale, 1))
            let slotW = Int((Double(geo.size.width) * scale).rounded())
            let bleed = ArtLayout.choose(width: width, height: height, slot: slotW) == "bleed"
            let capW = ArtLayout.cappedPoints(native: width, slotPoints: Double(geo.size.width), scale: scale)
            let capH = ArtLayout.cappedPoints(native: height, slotPoints: Double(geo.size.height), scale: scale)
            AsyncImage(url: url) { phase in
                if let image = phase.image {
                    picture(image, bleed: bleed, capW: capW, capH: capH, slot: geo.size)
                }
            }
            .frame(width: geo.size.width, height: geo.size.height)
            .clipped()
        }
        .accessibilityHidden(true)
    }

    @ViewBuilder
    private func picture(_ image: Image, bleed: Bool, capW: Double, capH: Double, slot: CGSize) -> some View {
        if bleed {
            image.resizable().scaledToFill()
                .frame(width: slot.width, height: slot.height)
        } else if hero {
            ZStack {
                image.resizable().scaledToFill()
                    .blur(radius: 22)
                    .opacity(0.45)
                image.resizable().scaledToFit()
                    .frame(
                        maxWidth: fitCap(capW, fraction: slot.width * 0.72, fallback: slot.width * 0.55),
                        maxHeight: fitCap(capH, fraction: slot.height * 0.82, fallback: slot.height * 0.82)
                    )
                    .clipShape(.rect(cornerRadius: Tokens.Radius.md))
            }
        } else {
            image.resizable().scaledToFit()
                .frame(
                    maxWidth: capW > 0 ? CGFloat(capW) : slot.width,
                    maxHeight: capH > 0 ? CGFloat(capH) : slot.height
                )
                .frame(width: slot.width, height: slot.height)
        }
    }

    private func fitCap(_ cap: Double, fraction: CGFloat, fallback: CGFloat) -> CGFloat {
        guard cap > 0 else { return fallback }
        return min(fraction, CGFloat(cap))
    }
}

/// Draws a still without going past 1.25× its native pixels. Unknown sides use the slot.
struct CappedFit: View {
    let image: Image
    var nativeWidth: Int
    var nativeHeight: Int

    @Environment(\.displayScale) private var displayScale

    var body: some View {
        GeometryReader { geo in
            let scale = Double(max(displayScale, 1))
            let capW = ArtLayout.cappedPoints(native: nativeWidth, slotPoints: Double(geo.size.width), scale: scale)
            let capH = ArtLayout.cappedPoints(native: nativeHeight, slotPoints: Double(geo.size.height), scale: scale)
            image.resizable().scaledToFit()
                .frame(
                    maxWidth: capW > 0 ? CGFloat(capW) : geo.size.width,
                    maxHeight: capH > 0 ? CGFloat(capH) : geo.size.height
                )
                .frame(width: geo.size.width, height: geo.size.height)
        }
        .accessibilityHidden(true)
    }
}

/// A picture to draw when the recording still is missing.
private struct PosterArt {
    var url: URL
    var width: Int
    var height: Int
}

/// Poster first. The still is 480 px wide. A missing poster uses program art, then the channel picture.
struct RecordingPoster: View {
    @Environment(AppStore.self) private var store
    let recording: Recording

    private static let posterWidth = 480

    var body: some View {
        AsyncImage(url: store.api?.posterURL(recordingID: recording.id)) { phase in
            if let image = phase.image {
                CappedFit(image: image, nativeWidth: Self.posterWidth, nativeHeight: 0)
            } else if phase.error != nil, let art = fallback {
                ProgramPicture(url: art.url, width: art.width, height: art.height)
                    .background(Tokens.ColorToken.surface2)
            } else {
                Tokens.ColorToken.surface2
            }
        }
        .accessibilityHidden(true)
    }

    private var fallback: PosterArt? {
        if let airing = store.index.artAiring(for: recording), let url = store.artURL(airing, width: Self.posterWidth) {
            return PosterArt(url: url, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
        }
        guard let channel = store.channels.first(where: { $0.id == recording.channelId }),
              let raw = channel.artUrl, !raw.isEmpty, let api = store.api else { return nil }
        return PosterArt(url: api.artURL(kind: "channel", id: channel.id, width: 320), width: channel.artWidth ?? 0, height: channel.artHeight ?? 0)
    }
}
