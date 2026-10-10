import SwiftUI

@main
struct BroadwaveWatchApp: App {
    @State private var phone = PhoneLink()

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                ScreensView()
            }
            .environment(phone)
        }
    }
}

/// The screens open on the server, televisions first.
struct ScreensView: View {
    @Environment(PhoneLink.self) private var phone
    @Environment(\.scenePhase) private var scenePhase
    @State private var opened: String?
    @State private var started = false

    var body: some View {
        List {
            // Rows from before an error stay, so the message goes above them.
            if let error = phone.error {
                Text(error)
                    .foregroundStyle(.secondary)
                Button("Try again") { phone.refresh() }
            } else if phone.screens.isEmpty, !phone.loading {
                Text("Open Broadwave on a TV or in a browser.")
                    .foregroundStyle(.secondary)
            }
            ForEach(phone.screens) { row in
                NavigationLink(value: row.id) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(row.name)
                            .lineLimit(1)
                        Text(row.playing.isEmpty ? "Not playing" : row.playing)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
            }
        }
        .navigationTitle("Broadwave")
        .navigationDestination(for: String.self) { RemoteView(screenID: $0) }
        .navigationDestination(item: $opened) { RemoteView(screenID: $0) }
        .overlay {
            if phone.loading, phone.screens.isEmpty {
                ProgressView()
            }
        }
        .task {
            // The list comes back here from each remote; ask once per launch.
            guard !started else { return }
            started = true
            phone.refresh()
            #if DEBUG
                await debugPress()
            #endif
        }
        // A raised wrist: what each screen plays has likely changed.
        .onChange(of: scenePhase) { _, phase in
            if phase == .active {
                phone.refresh()
            }
        }
    }

    #if DEBUG
        /// `-BroadwaveWatchPress down,pause,play` presses those buttons on the first
        /// screen that plays something, and `-BroadwaveWatchOpen YES` opens its remote,
        /// so a simulator run needs no taps.
        private func debugPress() async {
            let raw = UserDefaults.standard.string(forKey: "BroadwaveWatchPress") ?? ""
            let presses = raw.split(separator: ",").compactMap { WatchRelay.Press(rawValue: String($0)) }
            let open = UserDefaults.standard.bool(forKey: "BroadwaveWatchOpen")
            guard !presses.isEmpty || open else { return }
            for _ in 0 ..< 40 where phone.screens.first(where: { !$0.playing.isEmpty }) == nil {
                try? await Task.sleep(for: .milliseconds(500))
                if !phone.loading {
                    phone.refresh()
                }
            }
            guard let row = phone.screens.first(where: { !$0.playing.isEmpty }) else {
                print("broadwave watch press: no screen playing")
                return
            }
            if open {
                opened = row.id
            }
            // `-BroadwaveWatchBurst YES` sends them back to back, as a fast crown turn does.
            let gap = UserDefaults.standard.bool(forKey: "BroadwaveWatchBurst") ? 0.0 : 4.0
            for press in presses {
                print("broadwave watch press \(press.rawValue) on \(row.name)")
                phone.press(press, on: row.id)
                try? await Task.sleep(for: .seconds(gap))
            }
            try? await Task.sleep(for: .seconds(4))
            phone.refresh()
        }
    #endif
}

/// One screen's remote. The crown changes channel, one per detent; a double tap plays or pauses.
struct RemoteView: View {
    @Environment(PhoneLink.self) private var phone
    let screenID: String
    @State private var crown = 0.0
    @State private var settled = 0.0
    @State private var paused = false

    var body: some View {
        let row = phone.row(screenID)
        VStack(spacing: 8) {
            Text(row?.playing.isEmpty == false ? row?.playing ?? "" : "Not playing")
                .font(.headline)
                .lineLimit(2)
                .multilineTextAlignment(.center)
                .accessibilityAddTraits(.updatesFrequently)
            HStack {
                button("Channel down", "chevron.down", .down)
                button("Channel up", "chevron.up", .up)
            }
            HStack {
                Button {
                    paused.toggle()
                    phone.press(paused ? .pause : .play, on: screenID)
                } label: {
                    Image(systemName: paused ? "play.fill" : "pause.fill")
                }
                .accessibilityLabel(paused ? "Play" : "Pause")
                .handGestureShortcut(.primaryAction)
                Button {
                    phone.press(.record, on: screenID)
                } label: {
                    Image(systemName: "record.circle")
                        .foregroundStyle(.red)
                }
                .accessibilityLabel("Record")
            }
            if let error = phone.error {
                Text(error)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle(row?.name ?? "Screen")
        .focusable()
        .focusEffectDisabled()
        .digitalCrownRotation(
            detent: $crown, from: -10000, through: 10000, by: 1, sensitivity: .low,
            isContinuous: false, isHapticFeedbackEnabled: true,
            onChange: { _ in }, onIdle: changeChannel
        )
    }

    private func button(_ label: String, _ symbol: String, _ press: WatchRelay.Press) -> some View {
        Button {
            step(press, 1)
        } label: {
            Image(systemName: symbol)
        }
        .accessibilityLabel(label)
    }

    private func changeChannel() {
        guard let (press, count) = WatchRelay.crownSteps(from: settled, to: crown) else { return }
        settled = crown
        step(press, count)
    }

    private func step(_ press: WatchRelay.Press, _ count: Int) {
        for _ in 0 ..< count {
            phone.press(press, on: screenID)
        }
    }
}
