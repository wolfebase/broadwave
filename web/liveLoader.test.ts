import Hls from "hls.js";
import { afterEach, expect, test } from "vitest";
import { livePlaylistLoader } from "./src/lib/primeLevel.ts";

const level = "http://127.0.0.1/media/live/4/level.m3u8";
const withParts =
  "#EXTM3U\n#EXTINF:2.000,\nseg1.m4s\n#EXT-X-PART:DURATION=0.500,URI=\"old.m4s\"\n#EXTINF:2.000,\nseg2.m4s\n#EXT-X-PART:DURATION=0.500,URI=\"open.m4s\"\n";

type Success = (response: { data: unknown }, stats: object, context: object, details: unknown) => void;

const baseLoad = Hls.DefaultConfig.loader.prototype.load;

afterEach(() => {
  Hls.DefaultConfig.loader.prototype.load = baseLoad;
});

function answer(data: unknown) {
  Hls.DefaultConfig.loader.prototype.load = function (_context, _config, callbacks) {
    callbacks.onSuccess({ url: level, data, code: 200 }, { aborted: false }, _context, null);
  };
}

function load(data: unknown): unknown {
  answer(data);
  const Loader = livePlaylistLoader() as unknown as new (config: object) => {
    load: (context: object, config: object, callbacks: { onSuccess: Success }) => void;
  };
  let got: unknown;
  new Loader({}).load({ url: level, responseType: "text" }, {}, {
    onSuccess: (response) => {
      got = response.data;
    },
  });
  return got;
}

test("a live playlist drops parts that already have a segment", () => {
  const got = load(withParts);
  expect(typeof got).toBe("string");
  expect(got).not.toContain("old.m4s");
  expect(got).toContain("seg1.m4s");
  expect(got).toContain("seg2.m4s");
  expect(got).toContain('URI="open.m4s"');
});

test("a playlist with no parts, and a body that is not text, is left as the loader received it", () => {
  const plain = "#EXTM3U\r\n#EXTINF:2.0,\r\nseg.m4s\r\n";
  expect(load(plain)).toBe(plain);
  const bytes = new Uint8Array([1, 2, 3]);
  expect(load(bytes)).toBe(bytes);
});
