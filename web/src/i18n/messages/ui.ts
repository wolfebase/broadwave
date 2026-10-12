import type { Message } from "../runtime.ts";

export const uiMessages = {
  "ui.atsc3": "ATSC 3.0",
  "ui.atsc3Short": "3.0",
  "ui.live": "Live",
  "ui.willRecord": "Will record",
  "ui.recording": "Recording",
} as const satisfies Record<string, Message>;
