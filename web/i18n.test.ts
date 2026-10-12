import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { actionError } from "./src/app/actionError.ts";
import { captionTrackName, streamAudioLabel } from "./src/features/player/audioLabel.ts";
import { copyMessages } from "./src/i18n/messages/copy.ts";
import { guideMessages } from "./src/i18n/messages/guide.ts";
import { appPages } from "./src/i18n/pages.ts";
import { formatDateTime, formatRelative, formatTime, localeMode, setLocaleMode, shippedLocale, translate, weekdayShortNames } from "./src/i18n/runtime.ts";
import { recordingCountLabel } from "./src/lib/guide.ts";
import { copy } from "./src/strings.ts";
import { formatClock, lastSeenPhrase } from "./src/time.ts";

test("english stays the shipped language and pseudo doubles translated text", () => {
  assert.equal(shippedLocale, "en");
  assert.equal(localeMode(), "en");
  assert.equal(copy.sources.title, "Sources");
  assert.equal(copy.library.foundBreaks(0), "No breaks found.");
  assert.equal(copy.library.foundBreaks(1), "Found 1 break.");
  assert.equal(copy.library.foundBreaks(3), "Found 3 breaks.");
  setLocaleMode("pseudo");
  try {
    assert.equal(localeMode(), "pseudo");
    const title = copy.sources.title;
    assert.equal(title, "SourcesSources");
    assert.equal(title.length, "Sources".length * 2);
    const line = copy.library.foundBreaks(3);
    assert.equal(line, "Found 3 breaks.Found 3 breaks.");
    assert.equal(line.length, "Found 3 breaks.".length * 2);
    const named = copy.sources.offline("5 minutes ago");
    assert.equal(named, "Offline. Last seen 5 minutes ago.Offline. Last seen 5 minutes ago.");
  } finally {
    setLocaleMode("en");
  }
  assert.equal(copy.sources.title, "Sources");
});

test("plurals follow Intl.PluralRules and keep a none line for zero", () => {
  const rules = new Intl.PluralRules("en");
  assert.equal(rules.select(0), "other");
  assert.equal(rules.select(1), "one");
  assert.equal(rules.select(2), "other");
  assert.equal(recordingCountLabel(0), "0 recordings");
  assert.equal(recordingCountLabel(1), "1 recording");
  assert.equal(recordingCountLabel(4), "4 recordings");
  assert.equal(translate(guideMessages["guide.recordings"], undefined, 1), "1 recording");
  assert.equal(copyMessages["library.foundBreaks"].zero, "No breaks found.");
});

test("clocks and dates follow the browser locale, and last seen stays English", () => {
  const date = new Date(2026, 9, 4, 13, 5);
  const clock = { hour: "numeric", minute: "2-digit" } as const;
  assert.equal(formatClock(date), date.toLocaleTimeString([], clock));
  assert.equal(formatTime(date), date.toLocaleTimeString([], clock));
  const when = { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" } as const;
  assert.equal(formatDateTime(date, when), date.toLocaleString([], when));
  assert.equal(formatDateTime(date), date.toLocaleString());
  const now = Date.parse("2026-10-04T12:00:00Z");
  assert.equal(lastSeenPhrase("2026-10-04T11:59:40Z", now), "just now");
  assert.equal(lastSeenPhrase("2026-10-04T11:55:00Z", now), "5 minutes ago");
  assert.equal(lastSeenPhrase("2026-10-04T11:59:00Z", now), "1 minute ago");
  assert.equal(lastSeenPhrase("2026-10-04T11:00:00Z", now), "1 hour ago");
  assert.equal(lastSeenPhrase("2026-10-04T09:00:00Z", now), "3 hours ago");
  assert.equal(lastSeenPhrase("2026-10-03T12:00:00Z", now), "1 day ago");
  assert.equal(lastSeenPhrase("2026-10-01T12:00:00Z", now), "3 days ago");
  assert.equal(lastSeenPhrase(undefined, now), "");
  assert.equal(lastSeenPhrase("not-a-time", now), "");
  setLocaleMode("pseudo");
  try {
    assert.equal(formatTime(date), date.toLocaleTimeString([], clock));
    assert.equal(formatRelative(-5, "minute"), "5 minutes ago5 minutes ago");
    assert.equal(lastSeenPhrase("2026-10-04T11:59:40Z", now), "just nowjust now");
  } finally {
    setLocaleMode("en");
  }
});

test("weekday names come from Intl in Sunday-first order", () => {
  const names = [0, 1, 2, 3, 4, 5, 6].map((index) => new Intl.DateTimeFormat("en", { weekday: "short" }).format(new Date(2024, 0, 7 + index)));
  assert.deepEqual(weekdayShortNames(), names);
  assert.deepEqual(names, ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]);
});

test("failed actions, stream audio, and captions use the catalog", () => {
  assert.equal(actionError(new Error("The tuner is busy."), "shell.recordFailed"), "The tuner is busy.");
  assert.equal(actionError(new Error(""), "shell.recordFailed"), "");
  assert.equal(actionError("no", "shell.recordFailed"), "Recording did not start.");
  assert.equal(actionError(null, "shell.airingNotScheduled"), "That airing was not scheduled.");
  assert.equal(actionError(undefined, "shell.passNotRemoved"), "That pass was not removed.");
  assert.equal(actionError(0, "shell.noTuner"), "No tuner answered.");
  assert.equal(actionError(false, "shell.deviceNotRemoved"), "That device was not removed.");
  assert.equal(streamAudioLabel("copy", "AC-3"), "Original AC-3");
  assert.equal(streamAudioLabel("copy"), "Original ");
  assert.equal(streamAudioLabel("aac6"), "5.1 AAC");
  assert.equal(streamAudioLabel("ac3"), "5.1 AC-3");
  assert.equal(streamAudioLabel("aac"), "Stereo AAC");
  assert.equal(captionTrackName(), "English CC");
  const data = fs.readFileSync(new URL("./src/app/data.tsx", import.meta.url), "utf8");
  const player = fs.readFileSync(new URL("./src/features/player/LivePlayer.tsx", import.meta.url), "utf8");
  const search = fs.readFileSync(new URL("./src/features/search/Search.tsx", import.meta.url), "utf8");
  assert.equal(data.includes("Recording did not start."), false);
  assert.equal(player.includes("Stereo AAC"), false);
  assert.equal(search.includes("Recording every "), false);
  setLocaleMode("pseudo");
  try {
    assert.equal(actionError(null, "shell.recordFailed"), "Recording did not start.Recording did not start.");
    assert.equal(actionError(new Error("busy"), "shell.recordFailed"), "busy");
    assert.equal(streamAudioLabel("aac6"), "5.1 AAC5.1 AAC");
    assert.equal(streamAudioLabel("ac3"), "5.1 AC-35.1 AC-3");
    assert.equal(streamAudioLabel(undefined), "Stereo AACStereo AAC");
    assert.equal(captionTrackName(), "English CCEnglish CC");
  } finally {
    setLocaleMode("en");
  }
});

test("every shell route is rendered by the pseudo-locale check", () => {
  const app = fs.readFileSync(new URL("./src/app/App.tsx", import.meta.url), "utf8");
  const routes = new Set(appPages.map((page) => page.path.split("?")[0]));
  for (const match of app.matchAll(/path === "(\/[^"]*)"/g)) {
    assert.equal(routes.has(match[1]), true, match[1]);
  }
  assert.equal(routes.has("/"), true);
});

function walk(dir: string, out: string[]) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, out);
    else if (entry.name.endsWith(".tsx")) out.push(full);
  }
}

function untranslated(source: string): string[] {
  // Leave "://" alone so a placeholder URL is not eaten as a line comment.
  const text = source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:])\/\/.*$/gm, "$1");
  const found: string[] = [];
  // JSX text sits between a tag's closing ">" and the next tag. "=>" and "/>"
  // also end in ">" and are not the start of a text node.
  for (const match of text.matchAll(/>([^<>{}]*[A-Za-z][^<>{}]*)</g)) {
    const gt = match.index!;
    const before = text[gt - 1];
    if (before === "=" || before === "/") continue;
    // Only the ">" that ends an opening tag. A comparison or a closing tag
    // also ends in ">" and the bytes after it are code.
    let tag = -1;
    for (let i = gt - 1; i >= 0; i--) {
      if (text[i] === ">") break;
      if (text[i] === "<") {
        tag = i;
        break;
      }
    }
    if (tag < 0 || text[tag + 1] === "/") continue;
    const prev = text[tag - 1];
    if (prev && /[A-Za-z0-9_]/.test(prev)) continue;
    const raw = match[1];
    if (/[;]|=>|&&|\|\||\?\.\b|\b(const|let|return|function|import|type)\b/.test(raw)) continue;
    const value = raw.replace(/\s+/g, " ").trim();
    if (value) found.push(value);
  }
  for (const match of text.matchAll(/(?:aria-label|aria-description|aria-placeholder|placeholder|alt|title)=["']([^"']*[A-Za-z][^"']*)["']/g)) {
    if (/[$<>{}]/.test(match[1])) continue;
    found.push(match[1]);
  }
  return found;
}

test("user-visible text in the pages comes from the catalog", () => {
  const files: string[] = [];
  walk(path.join(path.dirname(new URL(import.meta.url).pathname), "src"), files);
  const hits: string[] = [];
  for (const file of files) {
    const found = untranslated(fs.readFileSync(file, "utf8"));
    for (const value of found) hits.push(`${path.relative("src", file)}: ${value}`);
  }
  assert.deepEqual(hits, []);
});

test("the Apple catalogs are English and hold the strings the apps look up", () => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const app = JSON.parse(fs.readFileSync(path.join(root, "apple/App/Localizable.xcstrings"), "utf8")) as {
    sourceLanguage: string;
    strings: Record<string, { localizations?: { en?: { stringUnit?: { value?: string } } } }>;
  };
  const kit = JSON.parse(fs.readFileSync(path.join(root, "apple/Packages/BroadwaveKit/Sources/BroadwaveKit/Localizable.xcstrings"), "utf8")) as {
    sourceLanguage: string;
    strings: Record<string, unknown>;
  };
  assert.equal(app.sourceLanguage, "en");
  assert.equal(kit.sourceLanguage, "en");
  assert.equal(app.strings["atsc.short"]?.localizations?.en?.stringUnit?.value, "3.0");
  for (const phrase of ["No listing", "No listings", "guide.channelName"]) {
    assert.ok(app.strings[phrase], phrase);
  }
  const missing: string[] = [];
  const kitSrc = path.join(root, "apple/Packages/BroadwaveKit/Sources/BroadwaveKit");
  for (const name of fs.readdirSync(kitSrc)) {
    if (!name.endsWith(".swift") || name === "SyncEngine.swift" || name === "WatchRelay.swift") continue;
    const source = fs.readFileSync(path.join(kitSrc, name), "utf8");
    for (const match of source.matchAll(/String\(localized:\s*"([^"\\]+)"/g)) {
      if (!kit.strings[match[1]]) missing.push(`${name}: ${match[1]}`);
    }
  }
  assert.deepEqual(missing, []);
});
