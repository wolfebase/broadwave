#if os(tvOS)
    import BroadwaveKit
    import BroadwaveUI
    import SwiftUI

    // The pieces every Apple TV page outside the guide and the player is built from:
    // a large title that scrolls away with the page, one chip, one poster card, one shelf.

    extension EnvironmentValues {
        /// True while the card button around this label has focus.
        @Entry var tvCardFocused = false
    }

    extension View {
        /// A page over the guide: no floating navigation title. The page draws its own
        /// large title, which scrolls away with the content.
        func tvPageChrome() -> some View {
            toolbar(.hidden, for: .navigationBar)
                .background(Tokens.ColorToken.canvas.ignoresSafeArea())
        }
    }

    /// The page's large title, first in its scroll view.
    struct TVPageTitle: View {
        let title: String
        var subtitle: String?

        init(_ title: String, subtitle: String? = nil) {
            self.title = title
            self.subtitle = subtitle
        }

        var body: some View {
            VStack(alignment: .leading, spacing: 6) {
                Text(title)
                    .font(.largeTitle.weight(.bold))
                    .accessibilityAddTraits(.isHeader)
                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.bottom, 8)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("page-title")
        }
    }

    /// A shelf heading, the same on every page.
    struct TVSectionTitle: View {
        let title: String
        var detail: String?

        init(_ title: String, detail: String? = nil) {
            self.title = title
            self.detail = detail
        }

        var body: some View {
            HStack(alignment: .firstTextBaseline, spacing: 16) {
                Text(title)
                    .font(.title3.weight(.semibold))
                    .accessibilityAddTraits(.isHeader)
                if let detail, !detail.isEmpty {
                    Text(detail)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    /// One horizontal row of cards. Up and Down move between shelves; Left and Right stay in the row.
    struct TVShelf<Content: View>: View {
        let title: String
        var detail: String?
        var spacing: CGFloat = 48
        @ViewBuilder var content: Content

        init(_ title: String, detail: String? = nil, spacing: CGFloat = 48, @ViewBuilder content: () -> Content) {
            self.title = title
            self.detail = detail
            self.spacing = spacing
            self.content = content()
        }

        var body: some View {
            VStack(alignment: .leading, spacing: 20) {
                TVSectionTitle(title, detail: detail)
                ScrollView(.horizontal) {
                    LazyHStack(alignment: .top, spacing: spacing) { content }
                        .padding(.vertical, 24)
                }
                .scrollIndicators(.hidden)
                .scrollClipDisabled()
                .focusSection()
            }
        }
    }

    /// The one chip on these pages: a filter, a recent search, an action in a state.
    /// Focused it turns white with dark text, as the system does; chosen it is lighter glass.
    struct TVChipStyle: ButtonStyle {
        var selected = false

        func makeBody(configuration: Configuration) -> some View {
            ChipBody(configuration: configuration, selected: selected)
        }

        private struct ChipBody: View {
            let configuration: ButtonStyleConfiguration
            let selected: Bool
            @Environment(\.isFocused) private var focused

            private var ink: Color {
                if focused {
                    return .black
                }
                return selected ? .white : .white.opacity(0.78)
            }

            private var fill: Color {
                if focused {
                    return .white
                }
                return .white.opacity(selected ? 0.26 : 0.09)
            }

            private var scale: CGFloat {
                if focused {
                    return 1.08
                }
                return configuration.isPressed ? 0.96 : 1
            }

            var body: some View {
                let ring: Double = selected && !focused ? 0.35 : 0
                configuration.label
                    .font(.callout.weight(.semibold))
                    .lineLimit(1)
                    .padding(.horizontal, 30)
                    .padding(.vertical, 14)
                    .foregroundStyle(ink)
                    .background(Capsule().fill(fill))
                    .overlay(Capsule().stroke(Color.white.opacity(ring), lineWidth: 2))
                    .scaleEffect(scale)
                    .shadow(color: .black.opacity(focused ? 0.45 : 0), radius: 14, y: 8)
                    .animation(.easeOut(duration: 0.16), value: focused)
            }
        }
    }

    /// A card that lifts its picture when focused and keeps its words under it, like the TV app.
    struct TVCardStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            CardBody(configuration: configuration)
        }

        private struct CardBody: View {
            let configuration: ButtonStyleConfiguration
            @Environment(\.isFocused) private var focused

            var body: some View {
                configuration.label
                    .environment(\.tvCardFocused, focused)
                    .scaleEffect(configuration.isPressed ? 0.97 : 1)
                    .zIndex(focused ? 1 : 0)
            }
        }
    }

    /// The picture part of a card: 16:9, rounded, lifted and lit when its button has focus.
    struct TVCardArt<Art: View>: View {
        var width: CGFloat
        @ViewBuilder var art: Art
        @Environment(\.tvCardFocused) private var focused

        var body: some View {
            art
                .frame(width: width, height: width * 9 / 16)
                .background(Tokens.ColorToken.surface2)
                .clipShape(.rect(cornerRadius: 16))
                .overlay(RoundedRectangle(cornerRadius: 16).stroke(.white.opacity(focused ? 0.9 : 0.08), lineWidth: focused ? 4 : 1))
                .scaleEffect(focused ? 1.1 : 1, anchor: .bottom)
                .shadow(color: .black.opacity(focused ? 0.6 : 0.2), radius: focused ? 30 : 6, y: focused ? 18 : 3)
                .animation(.spring(response: 0.3, dampingFraction: 0.8), value: focused)
        }
    }

    /// Words under a card. They move down a little when the picture above lifts.
    struct TVCardCaption: View {
        let title: String
        var line: String?
        var width: CGFloat
        @Environment(\.tvCardFocused) private var focused

        var body: some View {
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.callout.weight(.semibold))
                    .lineLimit(1)
                    .foregroundStyle(focused ? .primary : .secondary)
                if let line, !line.isEmpty {
                    Text(line)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .frame(width: width, alignment: .leading)
            .offset(y: focused ? 20 : 0)
            .animation(.spring(response: 0.3, dampingFraction: 0.8), value: focused)
        }
    }

    /// A small label on a card's picture: LIVE in red, or a quiet one such as a time.
    struct TVBadge: View {
        enum Tone {
            case live, quiet, bright
        }

        let text: String
        var tone: Tone = .quiet

        init(_ text: String, tone: Tone = .quiet) {
            self.text = text
            self.tone = tone
        }

        var body: some View {
            HStack(spacing: 8) {
                if tone == .live {
                    Circle().fill(.white).frame(width: 10, height: 10)
                }
                Text(text.uppercased())
                    .font(.caption2.weight(.heavy))
                    .tracking(1)
                    .monospacedDigit()
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .foregroundStyle(tone == .bright ? Color.black : Color.white)
            .background(background, in: .rect(cornerRadius: 8))
        }

        private var background: Color {
            switch tone {
            case .live: Tokens.ColorToken.tally
            case .quiet: Color.black.opacity(0.62)
            case .bright: Color.white
            }
        }
    }

    /// Program art, a live frame, or the channel number on a tinted field: never a blank card.
    struct TVProgramArt: View {
        @Environment(AppStore.self) private var store
        let channel: Channel?
        let airing: Airing?
        var width = 640

        var body: some View {
            if let airing, let url = store.artURL(airing, width: width) {
                ProgramPicture(url: url, width: airing.imageWidth ?? 0, height: airing.imageHeight ?? 0)
            } else if let channel, let frame = store.api?.frameURL(channelID: channel.id, width: width, listed: store.frameIDs) {
                AsyncImage(url: frame) { phase in
                    if let image = phase.image {
                        image.resizable().scaledToFill()
                    } else {
                        TVNumberField(number: channel.displayNumber, tint: (airing?.kind ?? .other).color)
                    }
                }
                .accessibilityHidden(true)
            } else {
                TVNumberField(number: channel?.displayNumber ?? "", tint: (airing?.kind ?? .other).color)
            }
        }
    }

    /// The channel number large on a tinted field, for a card with no picture.
    struct TVNumberField: View {
        let number: String
        let tint: Color

        var body: some View {
            ZStack {
                LinearGradient(colors: [tint.opacity(0.55), Tokens.ColorToken.surface2], startPoint: .topLeading, endPoint: .bottomTrailing)
                Text(number)
                    .font(.system(size: 64, weight: .heavy))
                    .monospacedDigit()
                    .foregroundStyle(.white.opacity(0.85))
                    .minimumScaleFactor(0.5)
                    .padding(20)
            }
            .accessibilityHidden(true)
        }
    }

    /// A message with a way forward. The buttons take focus, so the page never opens with nothing focused.
    struct TVEmptyState<Actions: View>: View {
        let symbol: String
        let title: String
        let message: String
        @ViewBuilder var actions: Actions

        var body: some View {
            VStack(spacing: 28) {
                Image(systemName: symbol)
                    .font(.system(size: 96, weight: .regular))
                    .foregroundStyle(.secondary)
                    .accessibilityHidden(true)
                VStack(spacing: 12) {
                    Text(title)
                        .font(.title2.weight(.bold))
                    Text(message)
                        .font(.body)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                        .frame(maxWidth: 1000)
                }
                .accessibilityElement(children: .combine)
                HStack(spacing: 32) { actions }
                    .focusSection()
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 60)
        }
    }
#endif
