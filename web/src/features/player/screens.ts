// The screens calls live with the player, not in api.ts, so the first page load does not carry them.
import { apiFailure, request } from "../../api";
import type { Screen } from "../../types";

export function getScreens() {
  return request<{ screens: Screen[] }>("/api/v1/screens");
}

// 202 has no body.
export async function sendToScreen(id: string, channelId: number, from: string) {
  const res = await fetch(`/api/v1/screens/${encodeURIComponent(id)}/watch`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ channelId, from }),
  });
  if (!res.ok) throw apiFailure(res.status, await res.text());
}

/** Polls until the screen is on the channel, up to 12 s. */
export async function startedOn(id: string, channelId: number, open: () => boolean) {
  const until = Date.now() + 12_000;
  while (open() && Date.now() < until) {
    await new Promise((done) => window.setTimeout(done, 1_000));
    try {
      const { screens } = await getScreens();
      if (screens.some((s) => s.id === id && s.channelId === channelId)) return true;
    } catch {
      // Ask again on the next second.
    }
  }
  return false;
}
