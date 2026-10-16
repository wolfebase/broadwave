import BroadwaveKit
import BroadwaveUI
import SwiftUI

/// The other open screens. Choosing one plays this channel there, and this one
/// stops once the server sees it playing there. A closed or sleeping screen
/// leaves the picture playing here and says so.
struct MoveToScreenList: View {
    @Environment(AppStore.self) private var store
    @Environment(NowPlaying.self) private var nowPlaying
    /// Called after the other screen started the channel.
    var onMoved: () -> Void

    @State private var screens: [Screen] = []
    @State private var loaded = false
    /// What the last move is doing, or why it failed.
    @State private var line: MoveLine?
    @State private var unreachable = false
    @State private var sending: String?

    var body: some View {
        content
            .task {
                // New screens show up while the list is open.
                while !Task.isCancelled {
                    await load()
                    try? await Task.sleep(for: .seconds(5))
                }
            }
            .onChange(of: line) { _, next in
                if let next {
                    AccessibilityNotification.Announcement(next.text).post()
                }
            }
            .accessibilityIdentifier("move-screens")
    }

    @ViewBuilder private var content: some View {
        #if os(tvOS)
            VStack(alignment: .leading, spacing: 16) {
                sharePlayRow
                if let line {
                    statusLine(line)
                }
                if !loaded {
                    ProgressView()
                        .accessibilityLabel("Looking for screens")
                } else if screens.isEmpty {
                    emptyLine
                } else {
                    ScrollView {
                        VStack(spacing: 4) {
                            ForEach(screens) { screen in
                                row(screen)
                            }
                        }
                        .padding(12)
                    }
                }
                Spacer(minLength: 0)
            }
            .padding(28)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        #else
            List {
                nowRow
                Section {
                    if let line {
                        statusLine(line)
                    }
                    if !loaded {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                            .accessibilityLabel("Looking for screens")
                    } else if screens.isEmpty {
                        noScreens
                    } else {
                        ForEach(screens) { screen in
                            row(screen)
                        }
                    }
                } header: {
                    Text("Broadwave screens")
                } footer: {
                    if !screens.isEmpty {
                        Text("It starts there and stops here.")
                    }
                }
                Section {
                    airPlayRow
                    sharePlayRow
                } header: {
                    Text("Other ways to watch")
                }
            }
        #endif
    }

    #if os(iOS)
        /// What is being moved.
        @ViewBuilder private var nowRow: some View {
            if let channel = nowPlaying.channel {
                let airing = store.index.on(channel.id, at: store.now)
                HStack(spacing: 14) {
                    Image(systemName: "dot.radiowaves.left.and.right")
                        .font(.title3)
                        .foregroundStyle(Tokens.ColorToken.tally)
                        .frame(width: 36)
                        .accessibilityHidden(true)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(airing?.title ?? channel.displayName)
                            .font(.headline)
                            .lineLimit(1)
                        Text("\(channel.displayNumber) \(channel.displayName)")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                }
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("move-now")
            }
        }

        private var noScreens: some View {
            ContentUnavailableView {
                Label(unreachable ? "Can't reach the server" : "No other screens", systemImage: unreachable ? "wifi.exclamationmark" : "tv.and.mediabox")
            } description: {
                Text(unreachable ? "Trying again." : "Open Broadwave on an Apple TV, iPad, iPhone, or a browser on this network and it shows up here.")
            }
            .accessibilityIdentifier("move-screens-empty")
        }

        /// An AirPlay TV or speaker, for a screen without Broadwave.
        private var airPlayRow: some View {
            HStack(spacing: 14) {
                AirPlayRoute(compact: true)
                    .frame(width: 44, height: 44)
                VStack(alignment: .leading, spacing: 2) {
                    Text("AirPlay")
                    Text("Send the picture to a TV or speaker that doesn't have Broadwave.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("move-airplay")
        }
    #endif

    /// In a FaceTime call, everyone on it can watch this channel too.
    @ViewBuilder private var sharePlayRow: some View {
        let sharePlay = nowPlaying.sharePlay
        let live = nowPlaying.together.isEmpty && nowPlaying.recordingScreens.isEmpty
        let invite = live ? WatchTogether.invite(server: store.server, channel: nowPlaying.channel) : nil
        if sharePlay.eligible || sharePlay.active, let invite {
            Button {
                if sharePlay.active {
                    sharePlay.leave()
                } else {
                    Task { await sharePlay.share(invite) }
                }
            } label: {
                HStack(spacing: 16) {
                    Image(systemName: "shareplay")
                        .frame(width: 36)
                        .accessibilityHidden(true)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(sharePlay.active ? "Leave SharePlay" : "SharePlay this channel")
                        Text(sharePlay.active ? "While you're in it, a channel change here changes it for everyone." : "Everyone in this call watches with you.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                    Spacer(minLength: 0)
                }
                #if os(tvOS)
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                #endif
                .contentShape(.rect)
            }
            #if os(tvOS)
            .buttonStyle(.plain)
            #endif
            .accessibilityIdentifier("move-shareplay")
        }
    }

    private var emptyLine: some View {
        Text(unreachable ? "Can't reach the server. Trying again." : "Open Broadwave on another screen and it shows up here.")
            .foregroundStyle(.secondary)
            .accessibilityIdentifier("move-screens-empty")
    }

    private func statusLine(_ line: MoveLine) -> some View {
        Text(line.text)
            .foregroundStyle(line.failed ? AnyShapeStyle(Tokens.ColorToken.tally) : AnyShapeStyle(.secondary))
            .accessibilityIdentifier(line.failed ? "move-screens-problem" : "move-screens-status")
    }

    private func row(_ screen: Screen) -> some View {
        let detail = watching(screen)
        let busy = sending == screen.id
        return Button {
            Task { await send(screen) }
        } label: {
            HStack(spacing: 16) {
                Image(systemName: symbol(screen.kind))
                    .frame(width: 36)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 2) {
                    Text(screen.name)
                        .lineLimit(1)
                    if let detail {
                        Text(detail)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                Spacer(minLength: 0)
                if busy {
                    ProgressView()
                        .accessibilityHidden(true)
                }
            }
            #if os(tvOS)
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            #endif
            .contentShape(.rect)
        }
        #if os(tvOS)
        .buttonStyle(.plain)
        #else
        // A disabled row would throw the remote's focus out of the panel, so Apple TV
        // keeps the rows and send() ignores a second press.
        .disabled(sending != nil)
        #endif
        .accessibilityLabel([screen.name, detail].compactMap(\.self).joined(separator: ", "))
        .accessibilityValue(busy ? "Starting" : "")
        .accessibilityHint("Plays this channel there and stops it here")
        .accessibilityIdentifier("move-screen-\(screen.id)")
    }

    /// "Watching 4.1", when the other screen plays a channel this one knows.
    private func watching(_ screen: Screen) -> String? {
        guard let id = screen.channelId, let channel = store.channels.first(where: { $0.id == id }) else { return nil }
        let number = channel.displayNumber.isEmpty ? channel.guideNumber : channel.displayNumber
        return "Watching \(number)"
    }

    private func symbol(_ kind: String) -> String {
        switch kind {
        case "appletv": "appletv"
        case "ipad": "ipad"
        case "iphone": "iphone"
        default: "desktopcomputer"
        }
    }

    private func load() async {
        guard let api = store.api else { return }
        do {
            let all = try await api.screens()
            screens = ScreenID.others(all, except: ScreenIdentity.id)
                .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
            unreachable = false
        } catch {
            unreachable = true
        }
        loaded = true
    }

    private func send(_ screen: Screen) async {
        guard sending == nil, let channel = nowPlaying.channel, nowPlaying.together.isEmpty, let api = store.api else { return }
        sending = screen.id
        line = nil
        do {
            try await api.sendToScreen(id: screen.id, channelId: channel.id, from: ScreenIdentity.name)
        } catch {
            sending = nil
            line = MoveLine(text: (error as? APIError)?.message ?? "Couldn't reach \(screen.name). Try again.", failed: true)
            await load()
            return
        }
        line = MoveLine(text: "Starting on \(screen.name)…", failed: false)
        let started = await MoveCheck.confirm(id: screen.id, channel: channel.id) {
            try? await api.screens()
        }
        sending = nil
        guard started else {
            line = MoveLine(text: "\(screen.name) didn't start it. It may be asleep.", failed: true)
            await load()
            return
        }
        line = nil
        // The viewer changed channel while the other screen started; this one keeps playing.
        guard nowPlaying.channel?.id == channel.id else { return }
        onMoved()
    }
}

private struct MoveLine: Equatable {
    var text: String
    var failed: Bool
}

#if os(iOS)
    /// The player's Move to another screen sheet.
    struct MoveToScreenSheet: View {
        @Environment(\.dismiss) private var dismiss
        var onMoved: () -> Void

        var body: some View {
            NavigationStack {
                MoveToScreenList(onMoved: onMoved)
                    .navigationTitle("Move to another screen")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button("Close", systemImage: "xmark") { dismiss() }
                                .accessibilityLabel("Close")
                        }
                    }
            }
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
            // A drag on the list resizes and closes the sheet before it scrolls.
            .presentationContentInteraction(.resizes)
        }
    }
#endif
