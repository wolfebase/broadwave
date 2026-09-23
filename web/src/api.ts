import type { Airing, Caps, Channel, ChannelPatch, Device, Pass, PlannedAiring, Prefs, Recording, ServerInfo, Settings, StorageInfo, TunerStatus, VirtualChannel, WatchSession } from "./types";

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
  return request<{ devices: Device[] }>("/api/devices");
}

export function discover(ip?: string) {
  return request<{ devices: Device[]; found: number }>("/api/sources/discover", {
    method: "POST",
    body: JSON.stringify({ ip: ip ?? "" }),
  });
}

export function getChannels(guide: boolean) {
  return request<{ channels: Channel[]; listings: string; message: string }>(
    `/api/channels${guide ? "?guide=1" : ""}`,
  );
}

export function patchChannel(id: number, patch: ChannelPatch) {
  return request<Channel>(`/api/channels/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function getSettings() {
  return request<Settings>("/api/settings");
}

export function getStorage() {
  return request<StorageInfo>("/api/storage");
}

export function putSettings(values: Partial<Settings>) {
  return request<Settings>("/api/settings", {
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

export function getServer() {
  return request<ServerInfo>("/api/v1/server");
}

export function getAirings() {
  return request<{ airings: Airing[] }>("/api/airings");
}

export function refreshGuide() {
  return request<{ airings: number }>("/api/guide/refresh", { method: "POST", body: "{}" });
}

export function getRecordings() {
  return request<{ recordings: Recording[] }>("/api/recordings");
}

export function startRecording(channelId: number, minutes: number, title: string) {
  return request<Recording>("/api/recordings", {
    method: "POST",
    body: JSON.stringify({ channelId, minutes, title }),
  });
}

export function stopRecording(id: number) {
  return request<{ ok: boolean }>(`/api/recordings/${id}/stop`, { method: "POST", body: "{}" });
}

export function getPasses() {
  return request<{ passes: Pass[] }>("/api/passes");
}

export function addPass(title: string, channelId: number) {
  return request<{ passes: Pass[] }>("/api/passes", {
    method: "POST",
    body: JSON.stringify({ title, channelId }),
  });
}

export function updatePass(id: number, patch: Partial<Pass>) {
  return request<{ passes: Pass[] }>(`/api/passes/${id}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function setWatched(id: number, watched: boolean) {
  return request<{ ok: boolean }>(`/api/recordings/${id}/watched`, {
    method: "PUT",
    body: JSON.stringify({ watched }),
  });
}

export function addSource(kind: string, name: string, url: string, xmltvUrl = "") {
  return request<{ id?: number; added?: number }>(`/api/sources`, {
    method: "POST",
    body: JSON.stringify({ kind, name, url, xmltvUrl }),
  });
}

export function updateVirtual(id: number, orderMode: string, ruleTitle: string) {
  return request<VirtualChannel>(`/api/virtuals/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ orderMode, ruleTitle }),
  });
}

export function deleteRecording(id: number) {
  return request<{ ok: boolean }>(`/api/recordings/${id}`, { method: "DELETE" });
}

export function deletePass(id: number) {
  return request<{ passes: Pass[] }>(`/api/passes/${id}`, { method: "DELETE" });
}

export function playRecording(id: number, pictureMode: string) {
  return request<{
    playlist: string;
    position: number;
    growing: boolean;
    markers: { id: number; recordingId: number; start: number; end: number }[];
  }>(`/api/recordings/${id}/play`, { method: "POST", body: JSON.stringify({ pictureMode }) });
}

export function saveProgress(id: number, position: number) {
  return request<{ position: number }>(`/api/recordings/${id}/progress`, {
    method: "PUT",
    body: JSON.stringify({ position }),
  });
}

export function deleteMarker(id: number) {
  return request<{ ok: boolean }>(`/api/markers/${id}`, { method: "DELETE" });
}

export function addMarker(id: number, start: number, end: number) {
  return request<{ id: number; start: number; end: number }>(`/api/recordings/${id}/markers`, {
    method: "POST",
    body: JSON.stringify({ start, end }),
  });
}

export function detectBreaks(id: number) {
  return request<{ markers: { id: number; start: number; end: number }[] }>(`/api/recordings/${id}/detect`, {
    method: "POST",
    body: "{}",
  });
}

export function getVirtuals() {
  return request<{ virtuals: VirtualChannel[] }>("/api/virtuals");
}

export function createVirtual(number: string, name: string, recordings: number[]) {
  return request<VirtualChannel>("/api/virtuals", {
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
  }>(`/api/virtuals/${id}/play`, {
    method: "POST",
    body: JSON.stringify({ index, pictureMode }),
  });
}

export function getTuners() {
  return request<{ tuners: TunerStatus[]; encoder: string }>("/api/tuners");
}

export function getSchedule() {
  return request<{ tunerCount: number; items: PlannedAiring[] }>("/api/schedule");
}

export function getEvents() {
  return request<{ events: { id: number; at: string; kind: string; message: string }[] }>("/api/events");
}
