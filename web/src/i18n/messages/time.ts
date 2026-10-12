import type { Message } from "../runtime";

export const timeMessages = {
  "time.justNow": "just now",
} as const satisfies Record<string, Message>;
