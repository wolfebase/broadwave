import type { Message } from "../runtime.ts";

export const castMessages = {
  "cast.cast": "Cast",
  "cast.stop": "Stop casting",
  "cast.didNotLoad": "Cast didn't load.",
  "cast.noDevices": "No Cast devices found.",
} as const satisfies Record<string, Message>;
