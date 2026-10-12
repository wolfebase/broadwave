import type { Message } from "../runtime.ts";

export const homeMessages = {
  "home.emptyTitle": "Let's find your tuner",
  "home.setup": "Set up",
  "home.emptyBody": "Connect an HDHomeRun to your network, then search for it.",
  "home.liveNow": "Live now",
  "home.watch": "Watch",
  "home.record": "Record",
  "home.nextAt": "Next at {time} · {title}",
  "home.recordingTitle": "Recording {title}",
  "home.onNow": "On now",
  "home.guide": "Guide",
  "home.onTheAir": "On the air",
  "home.yourTeams": "Your teams",
  "home.sports": "Sports",
  "home.watchTogether": "Watch together",
  "home.allSports": "All sports",
  "home.savedSets": "Saved sets",
  "home.continueWatching": "Continue watching",
  "home.tonight": "Tonight",
  "home.recent": "Recently recorded",
  "home.recordings": "Recordings",
} as const satisfies Record<string, Message>;
