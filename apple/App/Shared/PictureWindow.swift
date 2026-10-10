import AVKit
import BroadwaveKit
import BroadwaveUI
import os
import SwiftUI

#if os(iOS)
    /// System route button. The spoken name is AirPlay, and a tap opens the picker.
    struct AirPlayRoute: View {
        var compact = false
        var onDismiss: () -> Void = {}
        var onPresent: () -> Void = {}

        var body: some View {
            let radius: CGFloat = compact ? 22 : Tokens.Radius.lg
            AirPlayPicker(onPresent: onPresent, onDismiss: onDismiss)
                .frame(width: 44, height: 44)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: compact ? .center : .top)
                .padding(.top, compact ? 0 : 4)
                .frame(height: compact ? 44 : 64)
                .background {
                    // Glass behind the picker, not around it: the picker is the hit target.
                    RoundedRectangle(cornerRadius: radius, style: .continuous)
                        .fill(Color.clear)
                        .glassEffect(in: .rect(cornerRadius: radius))
                        .allowsHitTesting(false)
                }
                .overlay(alignment: .bottom) {
                    if !compact {
                        Text(PictureHandoff.airPlayLabel)
                            .font(.caption2.weight(.semibold))
                            .padding(.bottom, 6)
                            .allowsHitTesting(false)
                            .accessibilityHidden(true)
                    }
                }
        }
    }

    private struct AirPlayPicker: UIViewRepresentable {
        var onPresent: () -> Void
        var onDismiss: () -> Void

        func makeUIView(context: Context) -> AirPlayPickerView {
            let picker = AirPlayPickerView(frame: CGRect(x: 0, y: 0, width: 44, height: 44))
            picker.prioritizesVideoDevices = true
            picker.delegate = context.coordinator
            return picker
        }

        func updateUIView(_: AirPlayPickerView, context: Context) {
            context.coordinator.onPresent = onPresent
            context.coordinator.onDismiss = onDismiss
        }

        func makeCoordinator() -> Presenting {
            Presenting(onPresent: onPresent, onDismiss: onDismiss)
        }

        @MainActor
        final class Presenting: NSObject, AVRoutePickerViewDelegate {
            var onPresent: () -> Void
            var onDismiss: () -> Void

            init(onPresent: @escaping () -> Void, onDismiss: @escaping () -> Void) {
                self.onPresent = onPresent
                self.onDismiss = onDismiss
            }

            nonisolated func routePickerViewWillBeginPresentingRoutes(_: AVRoutePickerView) {
                MainActor.assumeIsolated { self.onPresent() }
            }

            nonisolated func routePickerViewDidEndPresentingRoutes(_: AVRoutePickerView) {
                MainActor.assumeIsolated { self.onDismiss() }
            }
        }
    }

    /// Names the inner button. The picker's own label is the symbol name until this runs.
    private final class AirPlayPickerView: AVRoutePickerView {
        override func layoutSubviews() {
            super.layoutSubviews()
            guard let button = subviews.compactMap({ $0 as? UIButton }).first,
                  button.accessibilityIdentifier != "airplay" else { return }
            button.accessibilityLabel = PictureHandoff.airPlayLabel
            button.accessibilityIdentifier = "airplay"
        }
    }

    /// Keeps the watch while Picture in Picture has the picture, and comes back to the same player.
    /// UIKit calls the delegate on the main thread but off the actor, so each method hops before touching state.
    @MainActor
    final class PictureDelegate: NSObject, AVPlayerViewControllerDelegate {
        var onChange: ((Bool) -> Void)?
        var onRestore: (() -> Void)?
        var onClosed: (() -> Void)?
        private var restoring = false
        /// The small window the system opened when the app left.
        private weak var awayWindow: AVPlayerViewController?
        private var comeBack: NSObjectProtocol?

        nonisolated func playerViewControllerWillStartPictureInPicture(_ vc: AVPlayerViewController) {
            MainActor.assumeIsolated {
                if UIApplication.shared.applicationState != .active {
                    self.watchComeBack(vc)
                }
                self.onChange?(true)
            }
        }

        private func watchComeBack(_ vc: AVPlayerViewController) {
            awayWindow = vc
            guard comeBack == nil else { return }
            comeBack = NotificationCenter.default.addObserver(
                forName: UIApplication.didBecomeActiveNotification, object: nil, queue: .main
            ) { [weak self] _ in
                MainActor.assumeIsolated { self?.endAwayWindow() }
            }
        }

        /// Back in the app, the picture belongs in the player again. A window
        /// the viewer opened from inside the app stays. AVKit has no stop call
        /// for this controller; turning the feature off and on ends the session.
        private func endAwayWindow() {
            guard let vc = awayWindow else { return }
            awayWindow = nil
            vc.allowsPictureInPicturePlayback = false
            vc.allowsPictureInPicturePlayback = true
        }

        private func forgetAwayWindow() {
            awayWindow = nil
            if let comeBack {
                NotificationCenter.default.removeObserver(comeBack)
            }
            comeBack = nil
        }

        nonisolated func playerViewController(_: AVPlayerViewController, failedToStartPictureInPictureWithError error: Error) {
            let text = error.localizedDescription
            MainActor.assumeIsolated {
                self.forgetAwayWindow()
                self.onChange?(false)
            }
            Logger(subsystem: "com.wolfeup.broadwave", category: "play").error("pip failed \(text, privacy: .public)")
        }

        nonisolated func playerViewControllerShouldAutomaticallyDismissAtPictureInPictureStart(_: AVPlayerViewController) -> Bool {
            PictureHandoff.dismissWhenPictureInPictureStarts
        }

        nonisolated func playerViewController(
            _: AVPlayerViewController,
            restoreUserInterfaceForPictureInPictureStopWithCompletionHandler completionHandler: @escaping (Bool) -> Void
        ) {
            MainActor.assumeIsolated {
                self.restoring = true
                self.awayWindow = nil
                self.onRestore?()
            }
            completionHandler(true)
        }

        nonisolated func playerViewControllerDidStopPictureInPicture(_: AVPlayerViewController) {
            MainActor.assumeIsolated {
                self.forgetAwayWindow()
                let away = UIApplication.shared.applicationState != .active
                let stop = PictureHandoff.stopWhenClosed(restored: self.restoring, away: away)
                self.restoring = false
                self.onChange?(false)
                if stop {
                    self.onClosed?()
                }
            }
        }
    }
#endif
