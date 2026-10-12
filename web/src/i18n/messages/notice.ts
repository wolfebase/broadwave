import type { Message } from "../runtime";

export const noticeMessages = {
  "notice.encrypted": "The 3.0 version is encrypted. Showing the regular broadcast.",
} as const satisfies Record<string, Message>;
