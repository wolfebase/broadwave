import { expect, test } from "vitest";
import type { Channel, Pass } from "./src/api/generated.ts";
import { daysLabel, passDetails, passKind, passLabel } from "./src/features/schedule/Passes.tsx";
import { formatClock } from "./src/time.ts";

function pass(extra: Partial<Pass> = {}): Pass {
  return { id: 1, title: "Den TV", ...extra };
}

function channel(id: number, number: string, name: string): Channel {
  return {
    id,
    deviceId: "duo",
    guideNumber: number,
    guideName: name,
    displayNumber: number,
    displayName: name,
    hd: true,
    favorite: false,
    enabled: true,
    hidden: false,
    present: true,
  };
}

function clock(hhmm: string) {
  const [h, m] = hhmm.split(":").map(Number);
  const date = new Date();
  date.setHours(h, m, 0, 0);
  return formatClock(date);
}

test("a pass is named by what it records", () => {
  const iso = "2026-10-11T18:00:00.000Z";
  const date = new Date(iso);
  const days = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  const when = `${days[date.getDay()]} ${date.getMonth() + 1}/${date.getDate()}, ${formatClock(date)}`;
  expect(passLabel(pass({ kind: "once", airingStart: iso, matchKind: "contains" }))).toBe(`Den TV · ${when} only`);
  expect(passLabel(pass({ kind: "once" }))).toBe("Den TV");
  expect(passLabel(pass({ kind: "team", matchKind: "category" }))).toBe("Den TV games");
  expect(passLabel(pass({ matchKind: "contains" }))).toBe("Titles with “Den TV”");
  expect(passLabel(pass({ matchKind: "category", title: "News" }))).toBe("News (category)");
  expect(passLabel(pass({ matchKind: "title" }))).toBe("Den TV");
  expect(passLabel(pass())).toBe("Den TV");
});

test("days collapse to weekdays, weekends, or the whole week", () => {
  expect(daysLabel([5, 1, 3, 2, 4])).toBe("Weekdays");
  expect(daysLabel([6, 0])).toBe("Weekends");
  expect(daysLabel([6, 5, 4, 3, 2, 1, 0])).toBe("Every day");
  expect(daysLabel([4, 2])).toBe("Tue, Thu");
  expect(daysLabel([])).toBe("");
});

test("the detail line keeps the rules a viewer would check", () => {
  const channels = [channel(4, "4.1", "Den TV")];
  // Two one-time recordings of one title tell apart by channel and time.
  expect(passDetails(pass({ kind: "once", channelId: 4, padBefore: 1, padAfter: 2 }), channels)).toBe("4.1 Den TV · Starts 1 min early, ends 2 min late");
  expect(passDetails(pass({ kind: "once", channelId: 9 }), channels)).toBe("");
  expect(passDetails(pass({ channelId: 9 }), channels)).toBe("Any channel");
  expect(passDetails(pass(), [])).toBe("Any channel");
  const full = pass({
    channelId: 4,
    days: [1, 2, 3, 4, 5],
    timeStart: "19:00",
    timeEnd: "22:30",
    episodes: "new",
    keepMode: "last",
    keepCount: 0,
    limitCount: 3,
    padBefore: 5,
    padAfter: 10,
  });
  expect(passDetails(full, channels)).toBe(
    `4.1 Den TV · Weekdays · ${clock("19:00")}–${clock("22:30")} · New only · Keeps the newest 1 · Pauses at 3 unwatched · Starts 5 min early, ends 10 min late`,
  );
  expect(passDetails(pass({ keepMode: "unwatched", timeStart: "19:00" }), [])).toBe("Any channel · Keeps unwatched");
});

test("each rule says its kind in plain words", () => {
  expect(passKind(pass())).toBe("Series");
  expect(passKind(pass({ kind: "series" }))).toBe("Series");
  expect(passKind(pass({ kind: "once" }))).toBe("One time");
  expect(passKind(pass({ kind: "manual" }))).toBe("Timed");
  expect(passKind(pass({ kind: "team" }))).toBe("Games");
});

test("a timed recording is named by its title or channel, and says when", () => {
  const channels = [channel(4, "4.1", "Den TV")];
  expect(passLabel(pass({ kind: "manual", title: "", channelId: 4 }), channels)).toBe("4.1 Den TV");
  expect(passLabel(pass({ kind: "manual", title: "Late news", channelId: 4 }), channels)).toBe("Late news");
  expect(passLabel(pass({ kind: "manual", title: "", channelId: 9 }), channels)).toBe("Timed");
  const once = pass({ kind: "manual", title: "", channelId: 4, date: "2026-10-21", timeStart: "22:00", duration: 90 });
  expect(passDetails(once, channels)).toBe(`4.1 Den TV · Wed 10/21 · ${clock("22:00")}–${clock("23:30")}`);
  const nightly = pass({ kind: "manual", title: "", channelId: 4, days: [1, 2, 3, 4, 5], timeStart: "23:30", duration: 60, keepMode: "last", keepCount: 3 });
  expect(passDetails(nightly, channels)).toBe(`4.1 Den TV · Weekdays · ${clock("23:30")}–${clock("00:30")} · Keeps the newest 3`);
});
