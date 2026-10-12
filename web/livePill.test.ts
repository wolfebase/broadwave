import { expect, test } from "vitest";

// LivePlayer reads the layout media queries while the module loads.
window.matchMedia = (query: string) =>
  ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener() {},
    removeEventListener() {},
    addListener() {},
    removeListener() {},
    dispatchEvent() {
      return false;
    },
  }) as MediaQueryList;

const { livePillLabel } = await import("./src/features/player/LivePlayer.tsx");

test("the pill says live until the picture is 14 seconds behind", () => {
  expect(livePillLabel(true, 90)).toBe("Live");
  expect(livePillLabel(false, 0)).toBe("Live");
  expect(livePillLabel(false, 13.9)).toBe("Live");
  expect(livePillLabel(false, 14)).toBe("14s behind");
  expect(livePillLabel(false, 59.4)).toBe("59s behind");
  expect(livePillLabel(false, 59.6)).toBe("1m 0s behind");
  expect(livePillLabel(false, 90)).toBe("1m 30s behind");
});
