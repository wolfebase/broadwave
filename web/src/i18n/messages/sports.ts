import type { Message } from "../runtime.ts";

export const sportsMessages = {
  "sports.title": "Sports",
  "sports.liveNow": "Live now",
  "sports.today": "Today",
  "sports.comingUp": "Coming up",
  "sports.watchTogether": "Watch together",
  "sports.everything": "Everything",
  "sports.noLive": "No games on right now",
  "sports.noGames": "No games in the guide",
  "sports.empty": "Sports on your channels show up here as soon as they're listed.",
  "sports.at": "at",
  "sports.watch": "Watch",
  "sports.record": "Record",
  "sports.recordAll": "Record all",
  "sports.everyGame": "Every {side} game",
  "sports.recordEveryGame": "Record every {side} game",
  "sports.follow": "Follow {side}",
  "sports.watchChannel": "Watch {channel}",
  "sports.notNow": "Not now",
} as const satisfies Record<string, Message>;
