import assert from "node:assert/strict";
import { test } from "node:test";
import { bracketHost, isLoopback, sharePageURL, shareRows, tunerAddress } from "./src/features/settings/tunerShare.ts";

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

test("opened at localhost, the share addresses use the server's home network address", () => {
  assert.equal(isLoopback("localhost"), true);
  assert.equal(isLoopback("127.0.0.1"), true);
  assert.equal(isLoopback("[::1]"), true);
  assert.equal(isLoopback("192.168.1.2"), false);
  const page = sharePageURL("http://localhost:8477/settings/integrations", "192.168.4.20:8478");
  assert.equal(page, "http://192.168.4.20:8477/settings/integrations");
  assert.equal(tunerAddress(page), "192.168.4.20:8478");
  // No home address known yet, or a page already on the network, stays as it is.
  assert.equal(sharePageURL("http://localhost:8477/x", ""), "http://localhost:8477/x");
  assert.equal(sharePageURL("http://localhost:8477/x", "localhost:8478"), "http://localhost:8477/x");
  assert.equal(sharePageURL("http://10.0.0.5:8477/x", "192.168.4.20:8478"), "http://10.0.0.5:8477/x");
});
