import assert from "node:assert/strict";
import { test } from "node:test";
import { bracketHost, shareRows, tunerAddress } from "./src/features/settings/tunerShare.ts";

test("each app gets the playlist, the guide, and the HDHomeRun address", () => {
  const rows = shareRows("http://tuner.example:8477/settings", true);
  const plex = rows.filter((row) => row.app === "Plex");
  assert.deepEqual(
    plex.map((row) => [row.label, row.value]),
    [
      ["HDHomeRun", "tuner.example:8478"],
      ["M3U playlist", "http://tuner.example:8477/export/lineup.m3u"],
      ["XMLTV guide", "http://tuner.example:8477/export/guide.xml"],
    ],
  );
  assert.equal(plex[0].hint, undefined);
  for (const app of ["Jellyfin", "Emby", "Channels"]) {
    assert.deepEqual(
      rows.filter((row) => row.app === app).map((row) => row.value),
      plex.map((row) => row.value),
    );
  }
});

test("https and an IPv6 host keep their shape", () => {
  const secure = shareRows("https://tuner.example/settings", false);
  assert.equal(secure[0].value, "tuner.example:8478");
  assert.equal(secure[0].hint, "Turn on Act as an HDHomeRun.");
  assert.equal(secure[1].value, "https://tuner.example/export/lineup.m3u");
  assert.equal(secure[2].value, "https://tuner.example/export/guide.xml");

  assert.equal(bracketHost("2001:db8::1"), "[2001:db8::1]");
  assert.equal(bracketHost("[2001:db8::1]"), "[2001:db8::1]");
  assert.equal(bracketHost("tuner.example"), "tuner.example");
  const v6 = shareRows("http://[2001:db8::1]:8477/settings", true);
  assert.equal(tunerAddress("http://[2001:db8::1]:8477/settings"), "[2001:db8::1]:8478");
  assert.equal(v6[0].value, "[2001:db8::1]:8478");
  assert.equal(v6[1].value, "http://[2001:db8::1]:8477/export/lineup.m3u");
});

test("the playlist and guide stay when the HDHomeRun switch is not offered", () => {
  const rows = shareRows("http://tuner.example:8477/settings", false, false);
  assert.equal(
    rows.some((row) => row.label === "HDHomeRun"),
    false,
  );
  assert.equal(rows.filter((row) => row.app === "Channels").length, 2);
  assert.equal(rows[0].value, "http://tuner.example:8477/export/lineup.m3u");
});
