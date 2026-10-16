import SwiftUI

/// What stands in for a picture that has not started: the channel's last frame,
/// fitted the way the picture will be, or the show's art blurred. Small art is
/// never stretched over the screen.
struct TuneBackdrop: View {
    let frame: URL?
    let art: URL?

    var body: some View {
        ZStack {
            Color.black
            if let frame {
                AsyncImage(url: frame) { phase in
                    if let image = phase.image {
                        image.resizable().scaledToFit()
                    }
                }
            } else if let art {
                AsyncImage(url: art) { phase in
                    if let image = phase.image {
                        image.resizable().scaledToFill()
                            .blur(radius: 28)
                            .opacity(0.75)
                    }
                }
            }
            Color.black.opacity(0.35)
        }
        .clipped()
        .accessibilityHidden(true)
        .allowsHitTesting(false)
    }
}
