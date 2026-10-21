import type { Message } from "../runtime";

/** The admin banner about the server's time zone. It loads with the banner, outside the first script. */
export const zoneMessages = {
  "zone.invalid": "This server's TZ is {zone}, which isn't a time zone, so it runs on UTC and records at the wrong hour. Set TZ to {suggest} and restart it.",
  "zone.unset": "This server has no time zone, so it runs on UTC and records at the wrong hour. Set TZ to {suggest} and restart it.",
  "zone.differs": "This server runs on {zone} time, {hours} h from this browser. If the antenna is here, set TZ to {suggest} and restart it.",
} as const satisfies Record<string, Message>;
