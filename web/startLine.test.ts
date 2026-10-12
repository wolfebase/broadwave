import { describe, expect, test } from "vitest";
import type { ChannelStart } from "./src/features/setup/useDiagnostics.ts";

// Stub before Diagnostics loads. These cases never render the page.
globalThis.fetch = (() => {
  throw new Error("startLine tests must not fetch");
}) as typeof fetch;

const { startLine } = await import("./src/features/setup/Diagnostics.tsx");

function row(extra: Partial<Pick<ChannelStart, "tune">> = {}): ChannelStart {
  return {
    at: "2026-10-11T18:00:00.000Z",
    channelId: 4,
    guideNumber: "4.1",
    rendition: "720",
    seconds: 3.75,
    keyframe: 1.25,
    encoder: 1,
    segment: 1,
    ...extra,
  };
}

const noTuner = "3.8\u00a0s · keyframe 1.3\u00a0s · encoder 1.0\u00a0s · first segment 1.0\u00a0s";

describe("startLine", () => {
  test("leaves the tuner out when the frequency was already tuned", () => {
    expect(startLine(row())).toBe(noTuner);
  });

  test("puts a positive tune second", () => {
    expect(startLine(row({ tune: 0.5 })).split(" · ")).toEqual([
      "3.8\u00a0s",
      "tuner 0.5\u00a0s",
      "keyframe 1.3\u00a0s",
      "encoder 1.0\u00a0s",
      "first segment 1.0\u00a0s",
    ]);
  });
});
