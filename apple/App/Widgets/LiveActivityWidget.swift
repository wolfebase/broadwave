import ActivityKit
import BroadwaveKit
import BroadwaveUI
import SwiftUI
import WidgetKit

/// The Lock Screen and Dynamic Island for a recording in progress or a followed game.
struct LiveActivityWidget: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: BroadwaveActivity.self) { context in
            LockScreenActivity(kind: context.attributes.kind, content: context.state)
                .padding(Tokens.Space.s3)
                .activityBackgroundTint(Tokens.ColorToken.canvas)
                .widgetURL(context.attributes.link)
        } dynamicIsland: { context in
            let kind = context.attributes.kind
            let content = context.state
            return DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    ActivityMark(kind: kind, live: content.live)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text(content.number).font(.caption.monospacedDigit().weight(.bold))
                }
                DynamicIslandExpandedRegion(.bottom) {
                    // The mark and the number are already in the regions above.
                    LockScreenActivity(kind: kind, content: content, header: false)
                }
            } compactLeading: {
                ActivityMark(kind: kind, live: content.live)
            } compactTrailing: {
                ActivityTrailing(kind: kind, content: content)
            } minimal: {
                ActivityMark(kind: kind, live: content.live)
            }
            .widgetURL(context.attributes.link)
        }
    }
}

/// A red dot while recording, a ball for a game.
struct ActivityMark: View {
    let kind: LiveActivityFeed.Kind
    let live: Bool

    var body: some View {
        switch kind {
        case .recording:
            Image(systemName: live ? "record.circle.fill" : "checkmark.circle")
                .foregroundStyle(live ? Tokens.ColorToken.tally : Tokens.ColorToken.textSecondary)
                .accessibilityLabel(live ? "Recording" : "Recorded")
        case .game:
            Image(systemName: "sportscourt.fill")
                .foregroundStyle(Tokens.ColorToken.accent)
                .accessibilityLabel(live ? "Game on now" : "Final")
        }
    }
}

/// The score for a game; time recorded for a recording.
struct ActivityTrailing: View {
    let kind: LiveActivityFeed.Kind
    let content: LiveActivityFeed.Content

    var body: some View {
        if kind == .game {
            Text(content.detail.isEmpty ? content.number : content.detail).font(.caption2.monospacedDigit()).lineLimit(1)
        } else if content.live {
            Text(timerInterval: content.start ... Date.distantFuture, countsDown: false)
                .font(.caption2.monospacedDigit())
                .frame(maxWidth: 52)
        } else {
            Text(content.number).font(.caption2.monospacedDigit())
        }
    }
}

struct LockScreenActivity: View {
    let kind: LiveActivityFeed.Kind
    let content: LiveActivityFeed.Content
    var header = true

    var body: some View {
        VStack(alignment: .leading, spacing: Tokens.Space.s1) {
            if header {
                HStack(spacing: Tokens.Space.s1) {
                    // The status text says it; the mark would say it twice to VoiceOver.
                    ActivityMark(kind: kind, live: content.live).accessibilityHidden(true)
                    Text(status).font(.caption.weight(.semibold)).foregroundStyle(Tokens.ColorToken.textSecondary)
                    Spacer(minLength: 0)
                    Text(content.number).font(.caption.monospacedDigit().weight(.bold))
                }
            }
            Text(content.title).font(.headline).lineLimit(1)
            if !content.detail.isEmpty {
                Text(content.detail).font(.subheadline.monospacedDigit()).lineLimit(1)
            }
            if content.live, let end = content.end, end > content.start {
                ProgressView(timerInterval: content.start ... end, countsDown: false) {
                    EmptyView()
                } currentValueLabel: {
                    EmptyView()
                }
                .tint(kind == .recording ? Tokens.ColorToken.tally : Tokens.ColorToken.accent)
            } else if content.live, kind == .recording {
                // No end to count toward: how long it has recorded.
                Text(timerInterval: content.start ... Date.distantFuture, countsDown: false)
                    .font(.subheadline.monospacedDigit())
                    .foregroundStyle(Tokens.ColorToken.textSecondary)
            }
        }
        .foregroundStyle(Tokens.ColorToken.text)
        .accessibilityElement(children: .combine)
    }

    private var status: String {
        switch (kind, content.live) {
        case (.recording, true): "Recording"
        case (.recording, false): "Recorded"
        case (.game, true): "On now"
        case (.game, false): "Final"
        }
    }
}
