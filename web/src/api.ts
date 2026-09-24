import type { Airing, Caps, Channel, ChannelPatch, Device, MultiviewPlan, Pass, PlannedAiring, Prefs, Recording, SearchAiring, ServerInfo, Settings, StorageInfo, TeamFollow, TunerStatus, VirtualChannel, WatchSession } from "./types";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    const text = await res.text();
    let message = text;
    try {
      const body = JSON.parse(text) as { message?: string };
      if (body.message) message = body.message;
    } catch {
      message = text;
    }
    throw new Error(message || res.statusText);
  }
  return res.json() as Promise<T>;
}

export function getDevices() {
  return request<{ devices: Device[] }>("/api/v1/devices");
}

export function startScan(deviceId: string) {
  return request<{ scanning: boolean }>(`/api/v1/devices/${encodeURIComponent(deviceId)}/scan`, {
    method: "POST",
    body: "{}",
  });
}

export type FreeFeed = { kind: string; name: string; addr: string; playlist: string; guide: string };

export function findFree() {
  return request<{ found: FreeFeed[]; guide: string }>("/api/v1/sources/free");
}

export function addFree(feed: { kind?: string; addr?: string; playlist?: string; guide?: string; name?: string }) {
  return request<SourceAdded>("/api/v1/sources/free", { method: "POST", body: JSON.stringify(feed) });
}

export type SourceStatus = {
  id: number;
  name: string;
  health?: string;
  refresh?: string;
  lastRefresh?: string;
  streamLimit?: number;
  streamsInUse?: number;
};

export function sourceStatuses() {
  return request<{ sources: SourceStatus[] }>("/api/v1/sources");
}

export function lookHarder() {
  return request<{ found: { kind: string; name: string; addr: string; id?: string }[] }>("/api/v1/sources/look", {
    method: "POST",
    body: "{}",
  });
}

export function discover(ip?: string) {
  return request<{ devices: Device[]; found: number }>("/api/v1/sources/discover", {
    method: "POST",
    body: JSON.stringify({ ip: ip ?? "" }),
  });
}

export function getChannels(guide: boolean) {
  return request<{ channels: Channel[]; listings: string; message: string }>(
    `/api/v1/channels${guide ? "?guide=1" : ""}`,
  );
}

export function patchChannel(id: number, patch: ChannelPatch) {
  return request<Channel>(`/api/v1/channels/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function getSettings() {
  return request<Settings>("/api/v1/settings");
}

export function getStorage() {
  return request<StorageInfo>("/api/v1/storage");
}

export function putSettings(values: Partial<Settings>) {
  return request<Settings>("/api/v1/settings", {
    method: "PUT",
    body: JSON.stringify(values),
  });
}

export function watchChannel(channelId: number, caps: Caps, prefs: Prefs, rendition = "") {
  return request<WatchSession>("/api/v1/watch", {
    method: "POST",
    body: JSON.stringify({ channelId, caps, prefs, rendition }),
  });
}

export function stopWatch(channelId: number, rendition = "") {
  return request<{ ok: boolean }>(`/api/v1/watch/${channelId}/stop`, { method: "POST", body: JSON.stringify({ rendition }) });
}

export function planMultiview(channelIds: number[]) {
  return request<MultiviewPlan>("/api/v1/multiview/plan", {
    method: "POST",
    body: JSON.stringify({ channelIds }),
  });
}

export function getServer() {
  return request<ServerInfo>("/api/v1/server");
}

export function search(q: string) {
  return request<{ query: string; airings: SearchAiring[]; recordings: Recording[] }>(`/api/v1/search?q=${encodeURIComponent(q)}`);
}

export function getAirings(window?: { from?: string; to?: string; channels?: string }) {
  const q = new URLSearchParams();
  if (window?.from) q.set("from", window.from);
  if (window?.to) q.set("to", window.to);
  if (window?.channels) q.set("channels", window.channels);
  const s = q.toString();
  return request<{ airings: Airing[] }>(`/api/v1/airings${s ? `?${s}` : ""}`);
}

export function refreshGuide() {
  return request<{ airings: number }>("/api/v1/guide/refresh", { method: "POST", body: "{}" });
}

export function getRecordings() {
  return request<{ recordings: Recording[] }>("/api/v1/recordings");
}

export function startRecording(channelId: number, minutes: number, title: string) {
  return request<Recording>("/api/v1/recordings", {
    method: "POST",
    body: JSON.stringify({ channelId, minutes, title }),
  });
}

export function stopRecording(id: number) {
  return request<{ ok: boolean }>(`/api/v1/recordings/${id}/stop`, { method: "POST", body: "{}" });
}

export function getTeams() {
  return request<{ teams: TeamFollow[] }>("/api/v1/teams");
}

export function followTeam(team: TeamFollow) {
  return request<{ teams: TeamFollow[] }>("/api/v1/teams", {
    method: "PUT",
    body: JSON.stringify(team),
  });
}

export function getPasses() {
  return request<{ passes: Pass[] }>("/api/v1/passes");
}

export function addPass(title: string, channelId: number) {
  return request<{ passes: Pass[] }>("/api/v1/passes", {
    method: "POST",
    body: JSON.stringify({ title, channelId }),
  });
}

export function updatePass(id: number, patch: Partial<Pass>) {
  return request<{ passes: Pass[] }>(`/api/v1/passes/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function setWatched(id: number, watched: boolean) {
  return request<{ ok: boolean }>(`/api/v1/recordings/${id}/watched`, {
    method: "PUT",
    body: JSON.stringify({ watched }),
  });
}

export function addSource(kind: string, name: string, url: string, xmltvUrl = "", groups = "", keep = "", username = "", password = "") {
  return request<SourceAdded>(`/api/v1/sources`, {
    method: "POST",
    body: JSON.stringify({ kind, name, url, xmltvUrl, groups, keep, username, password }),
  });
}

export type SourceAdded = {
  id?: number;
  added?: number;
  pick?: boolean;
  message?: string;
  groups?: string[];
  channels?: { name: string; id: string; number: string }[];
};

export function addPlaylistFile(name: string, groups: string, file: File, keep = "") {
  const body = new FormData();
  body.set("name", name);
  body.set("groups", groups);
  body.set("keep", keep);
  body.set("file", file);
  return fetch("/api/v1/sources", { method: "POST", body }).then(async (res) => {
    const text = await res.text();
    if (!res.ok) {
      let message = text;
      try {
        const parsed = JSON.parse(text) as { message?: string };
        if (parsed.message) message = parsed.message;
      } catch {
        message = text;
      }
      throw new Error(message || res.statusText);
    }
    return JSON.parse(text) as SourceAdded;
  });
}

export function updateVirtual(id: number, orderMode: string, ruleTitle: string) {
  return request<VirtualChannel>(`/api/v1/virtuals/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ orderMode, ruleTitle }),
  });
}

export function deleteRecording(id: number) {
  return request<{ ok: boolean }>(`/api/v1/recordings/${id}`, { method: "DELETE" });
}

export function deletePass(id: number) {
  return request<{ passes: Pass[] }>(`/api/v1/passes/${id}`, { method: "DELETE" });
}

export function playRecording(id: number, pictureMode: string) {
  return request<{
    playlist: string;
    position: number;
    growing: boolean;
    markers: { id: number; recordingId: number; start: number; end: number }[];
  }>(`/api/v1/recordings/${id}/play`, { method: "POST", body: JSON.stringify({ pictureMode }) });
}

export function saveProgress(id: number, position: number) {
  return request<{ position: number }>(`/api/v1/recordings/${id}/progress`, {
    method: "PUT",
    body: JSON.stringify({ position }),
  });
}

export function deleteMarker(id: number) {
  return request<{ ok: boolean }>(`/api/v1/markers/${id}`, { method: "DELETE" });
}

export function addMarker(id: number, start: number, end: number) {
  return request<{ id: number; start: number; end: number }>(`/api/v1/recordings/${id}/markers`, {
    method: "POST",
    body: JSON.stringify({ start, end }),
  });
}

export function detectBreaks(id: number) {
  return request<{ markers: { id: number; start: number; end: number }[] }>(`/api/v1/recordings/${id}/detect`, {
    method: "POST",
    body: "{}",
  });
}

export function getVirtuals() {
  return request<{ virtuals: VirtualChannel[] }>("/api/v1/virtuals");
}

export function createVirtual(number: string, name: string, recordings: number[]) {
  return request<VirtualChannel>("/api/v1/virtuals", {
    method: "POST",
    body: JSON.stringify({ number, name, recordings }),
  });
}

export function playVirtual(id: number, index: number, pictureMode = "broadcast") {
  return request<{
    usesTuner: boolean;
    index: number;
    count: number;
    playlist: string;
    number: string;
    name: string;
    recording: Recording;
    markers: { id: number; recordingId: number; start: number; end: number }[];
  }>(`/api/v1/virtuals/${id}/play`, {
    method: "POST",
    body: JSON.stringify({ index, pictureMode }),
  });
}

export function getTuners() {
  return request<{ tuners: TunerStatus[]; encoder: string }>("/api/v1/tuners");
}

export function getSchedule() {
  return request<{ tunerCount: number; items: PlannedAiring[] }>("/api/v1/schedule");
}

export function getEvents() {
  return request<{ events: { id: number; at: string; kind: string; message: string }[] }>("/api/v1/events");
}
