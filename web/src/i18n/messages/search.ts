import type { Message } from "../runtime.ts";

export const searchMessages = {
  "search.title": "Search",
  "search.placeholder": "Shows, people, recordings",
  "search.label": "Search shows, people, and recordings",
  "search.guide": "Guide",
  "search.recordings": "Recordings",
  "search.recordEvery": "Record every airing",
  "search.nothing": "Nothing matches.",
  "search.failed": "Search failed.",
  "search.recordingEvery": "Recording every {title}.",
} as const satisfies Record<string, Message>;
