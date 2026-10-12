import assert from "node:assert/strict";
import { test } from "node:test";
import { copy } from "./src/strings.ts";
import { choosePrimeURL, playbackURL, primedSounds, quietSubtitles, showCaptions, takePrimed } from "./src/lib/playbackURL.ts";

const index = "/media/live/4/1080.aac2.broadcast/index.m3u8";
const master = "/media/live/4/1080.aac2.broadcast/master.m3u8";
const main = "/media/live/4/1080.aac2.broadcast/main.m3u8";
const ladder = "/media/live/4/1080.aac2.broadcast.abr/ladder.m3u8";

// What the player did before the ladder, for a watch that is not one.
const before = {
  prime: (s: { mainPlaylist?: string }, alternates: boolean) => (alternates && /\/master\.m3u8(\?|$)/.test(s.mainPlaylist ?? "") ? s.mainPlaylist! : ""),
  level: (levels: { url: string }[]) => {
    const urls = new Set(levels.map((level) => level.url));
    return urls.size === 1 ? [...urls][0] : null;
  },
};

test("without a ladder, the master is primed only with alternates, and the index plays otherwise", () => {
  for (const mainPlaylist of [undefined, "", master, `${master}?v=1`, main, index, "/media/live/4/x/master.m3u8.bak"]) {
    for (const alternates of [false, true]) {
      const session = { playlist: index, mainPlaylist };
      const got = playbackURL(session, alternates);
      assert.equal(got.ladder, false);
      assert.equal(got.prime, before.prime(session, alternates), `${mainPlaylist} ${alternates}`);
      assert.equal(got.src, index, `${mainPlaylist} ${alternates}`);
    }
  }
});

test("without a ladder, only a master with one level URL is primed", () => {
  const cases: { url: string; height?: number }[][] = [
    [],
    [{ url: "/a/main.m3u8" }],
    [{ url: "/a/main.m3u8" }, { url: "/a/main.m3u8", height: 1080 }],
    [{ url: "/a/1080.m3u8", height: 1080 }, { url: "/a/720.m3u8", height: 720 }],
    [{ url: "/a/a.m3u8" }, { url: "/a/b.m3u8" }, { url: "/a/a.m3u8" }],
  ];
  for (const levels of cases) assert.equal(choosePrimeURL(levels, false), before.level(levels), JSON.stringify(levels));
});

test("without a ladder, sounds are set exactly when the master was primed", () => {
  assert.equal(primedSounds(null, false), null);
  assert.deepEqual(primedSounds({ sounds: [] }, false), []);
  assert.deepEqual(primedSounds({ sounds: [{ name: "English" }] }, false), [{ name: "English" }]);
});

test("without a ladder, captions follow the main playlist and subtitles are left alone", () => {
  for (const mainPlaylist of [undefined, "", master, main, index]) assert.equal(showCaptions(mainPlaylist), !!mainPlaylist);
  assert.equal(quietSubtitles(false, false), false);
  assert.equal(quietSubtitles(true, false), false);
});

test("without a ladder, a primed body is taken once and the others stay", () => {
  const bodies = new Map([
    ["m", "master"],
    ["l", "level"],
  ]);
  assert.equal(takePrimed(bodies, "m", "audio", false), undefined);
  assert.equal(bodies.get("l"), "level");
  assert.equal(takePrimed(bodies, "m", "l", false), "level");
  assert.equal(takePrimed(bodies, "m", "l", false), undefined);
  assert.equal(bodies.get("m"), "master");
});

test("a ladder plays and primes itself, with or without alternates", () => {
  for (const alternates of [false, true]) {
    assert.deepEqual(playbackURL({ playlist: index, mainPlaylist: ladder }, alternates), { prime: ladder, src: ladder, ladder: true });
  }
  const query = `${ladder}?v=1`;
  assert.deepEqual(playbackURL({ playlist: index, mainPlaylist: query }, false), { prime: query, src: query, ladder: true });
});

test("a ladder primes its tallest video level", () => {
  assert.equal(
    choosePrimeURL(
      [
        { url: "/l/720/index.m3u8", height: 720 },
        { url: "/l/1080/index.m3u8", height: 1080 },
        { url: "/l/audio-2.m3u8", height: 0, videoCodec: "mp4a.40.2" },
      ],
      true,
    ),
    "/l/1080/index.m3u8",
  );
  assert.equal(choosePrimeURL([{ url: "/l/1080/index.m3u8", height: 1080 }], true), "/l/1080/index.m3u8");
  assert.equal(choosePrimeURL([{ url: ["/l/a.m3u8", "/l/b.m3u8"] }], true), "/l/a.m3u8");
  assert.equal(choosePrimeURL([], true), null);
  assert.equal(
    choosePrimeURL(
      [
        { url: "/l/low.m3u8", bitrate: 64000, videoCodec: "mp4a.40.2" },
        { url: "/l/high.m3u8", bitrate: 128000, videoCodec: "mp4a.40.2" },
      ],
      true,
    ),
    "/l/high.m3u8",
  );
});

test("a ladder with no sound tracks watches again for another sound", () => {
  assert.equal(primedSounds({ sounds: [] }, true), null);
  assert.deepEqual(primedSounds({ sounds: [{ name: "English" }] }, true), [{ name: "English" }]);
});

test("an unprimed ladder leaves subtitles to the player", () => {
  assert.equal(quietSubtitles(false, true), true);
  assert.equal(quietSubtitles(true, true), false);
});

test("a ladder offers captions only when its master lists them", () => {
  assert.equal(showCaptions(ladder, '#EXT-X-STREAM-INF:SUBTITLES="cc"\n'), true);
  assert.equal(showCaptions(ladder, "#EXTM3U\n"), false);
  assert.equal(showCaptions(ladder), false);
});

test("a ladder drops the primed rung once the player starts on another", () => {
  const m = "http://live.example/ladder.m3u8";
  const tall = "http://live.example/1080/index.m3u8";
  const short = "http://live.example/480/index.m3u8";
  const kept = new Map([
    [m, "master"],
    [tall, "old"],
  ]);
  assert.equal(takePrimed(kept, m, tall, true), "old");
  assert.equal(kept.has(tall), false);
  assert.equal(kept.get(m), "master");

  const dropped = new Map([
    [m, "master"],
    [tall, "old"],
  ]);
  assert.equal(takePrimed(dropped, m, m, true), "master");
  assert.equal(takePrimed(dropped, m, short, true), undefined);
  assert.equal(dropped.has(tall), false);
});

test("the adaptive hint does not promise a browser will drop to sound only", () => {
  assert.match(copy.settings.adaptiveHint, /Apple can also drop to sound only/);
});
