import { expect, test } from "vitest";
import { applySettings } from "./src/features/player/liveCaptions.ts";

// jsdom does not provide VTTCue. This stand-in has the fields the player writes.
class Cue {
  snapToLines = true;
  line: number | "auto" = "auto";
  position: number | "auto" = "auto";
  size = 100;
  align: AlignSetting = "center";
  constructor(
    public startTime: number,
    public endTime: number,
    public text: string,
  ) {}
}

function cue() {
  return new Cue(0, 1, "Den TV") as unknown as VTTCue;
}

test("cue settings land on the caption, and a bad value is left alone", () => {
  const one = cue();
  applySettings(one, "line:90% position:40,line size:80 align:start");
  expect(one.snapToLines).toBe(false);
  expect(one.line).toBe(90);
  expect(one.position).toBe(40);
  expect(one.size).toBe(80);
  expect(one.align).toBe("start");

  const rows = cue();
  applySettings(rows, "line:2 align:end");
  expect(rows.snapToLines).toBe(true);
  expect(rows.line).toBe(2);
  expect(rows.align).toBe("end");

  const ignored = cue();
  ignored.line = 4;
  ignored.align = "center";
  applySettings(ignored, "line:nope align:top");
  expect(ignored.line).toBe(4);
  expect(ignored.align).toBe("center");

  for (const align of ["center", "left", "right"] as const) {
    const next = cue();
    applySettings(next, `align:${align}`);
    expect(next.align).toBe(align);
  }
});
