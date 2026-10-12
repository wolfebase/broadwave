// Cast receiver. The CAF script is on the page, from Google's CDN. This file
// plays HLS and joins the channel's sync room on its own socket.

import { reconnectWait } from "../lib/events.ts";
import { hlsCastPlayer } from "./hlsPlayer.ts";
import { castLine, castNamespace, parseLoad, type CastLoad } from "./protocol.ts";
import { castSessionLive, openCastSession, type CastPlayer, type CastSession, type CastStatus, type CastTransport } from "./session.ts";
import { createCastHold, type CastHold } from "./watch.ts";

type CastMessage = { data?: unknown };

type ReceiverContext = {
  start(options: ReceiverOptions): void;
  addCustomMessageListener(namespace: string, listener: (event: CastMessage) => void): void;
};

type ReceiverOptions = {
  skipPlayersLoad: boolean;
  disableIdleTimeout: boolean;
  maxInactivity: number;
  customNamespaces: Record<string, string>;
};

type CastGlobal = {
  framework?: {
    CastReceiverContext: { getInstance(): ReceiverContext };
    CastReceiverOptions: new () => ReceiverOptions;
    system?: { MessageType?: { JSON?: string } };
  };
};

type SocketBox = { ws: WebSocket | null };

const box: SocketBox = { ws: null };
let session: CastSession | null = null;
let player: CastPlayer | null = null;
let channelHold: CastHold | null = null;
let retry = 0;
let tickTimer = 0;
let clockTimer = 0;
let waitTimer = 0;
const burstTimers: number[] = [];

function paint(status: CastStatus) {
  const root = document.documentElement;
  root.dataset.castReceiver = "";
  root.dataset.castState = status;
  const idle = document.getElementById("cast-idle");
  const title = document.getElementById("cast-title");
  if (!idle || !title) return;
  const text = castLine(session?.title ?? "", session?.subtitle ?? "");
  if (status === "idle") {
    idle.hidden = false;
    title.hidden = true;
    return;
  }
  idle.hidden = true;
  if (status === "playing" || text === "") {
    title.hidden = true;
    return;
  }
  title.hidden = false;
  title.textContent = text;
}

function transport(): CastTransport {
  return {
    send(message) {
      const ws = box.ws;
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(message));
    },
    close() {
      box.ws?.close();
    },
  };
}

function clearBurst() {
  for (const id of burstTimers.splice(0)) window.clearTimeout(id);
}

function armTicks(current: CastSession) {
  window.clearInterval(tickTimer);
  window.clearInterval(clockTimer);
  tickTimer = window.setInterval(() => current.tick(), 250);
  clockTimer = window.setInterval(() => current.sampleClock(), 15_000);
}

function armWait(current: CastSession) {
  window.clearTimeout(waitTimer);
  const generation = current.generation;
  waitTimer = window.setTimeout(() => current.markWaitOver(generation), 800);
}

function connect(current: CastSession) {
  const ws = new WebSocket(current.socketURL());
  box.ws = ws;
  ws.onopen = () => {
    if (box.ws !== ws) return;
    retry = 0;
    current.onOpen();
    clearBurst();
    for (let i = 1; i < 5; i++) burstTimers.push(window.setTimeout(() => current.sampleClock(), i * 300));
  };
  ws.onmessage = (event) => {
    if (box.ws !== ws) return;
    try {
      current.onMessage(JSON.parse(String(event.data)));
    } catch {
      // A malformed frame is ignored, same as the web player.
    }
  };
  ws.onclose = () => {
    if (box.ws !== ws || current.stopped) return;
    window.setTimeout(() => connect(current), reconnectWait(retry++));
  };
}

function reconnect(current: CastSession) {
  const old = box.ws;
  box.ws = null;
  old?.close();
  connect(current);
}

function hold(): CastHold {
  channelHold ??= createCastHold(location.href);
  return channelHold;
}

function accept(load: CastLoad) {
  const video = document.querySelector<HTMLVideoElement>("#cast-video");
  if (!video) return;
  if (!player) player = hlsCastPlayer(video);
  if (!castSessionLive(session)) {
    session = null;
    const opened = openCastSession({
      pageURL: location.href,
      channelId: load.channelId,
      playlist: load.playlist,
      room: load.room,
      latency: load.latency,
      token: load.token,
      title: load.title,
      subtitle: load.subtitle,
      player,
      transport: transport(),
      screenId: screenId(),
      onStatus: paint,
      watch: hold(),
    });
    if (!opened) return;
    session = opened;
    paint(opened.status);
    connect(opened);
    armTicks(opened);
    armWait(opened);
    return;
  }
  const reopen = session.retune(load);
  paint(session.status);
  if (reopen) reconnect(session);
  armWait(session);
}

function onCastMessage(event: CastMessage) {
  let data = event.data;
  if (typeof data === "string") {
    try {
      data = JSON.parse(data);
    } catch {
      return;
    }
  }
  if (!data || typeof data !== "object") return;
  if ((data as { type?: unknown }).type === "stop") {
    session?.stop();
    return;
  }
  const load = parseLoad(data);
  if (load) accept(load);
}

function screenId(): string {
  const cryptoObj = globalThis.crypto;
  if (cryptoObj?.randomUUID) return cryptoObj.randomUUID();
  return `cast-${Date.now().toString(16)}`;
}

/** Starts the receiver when the Cast framework is on the page. A normal browser stays on the idle line. */
export function bootCastReceiver(cast: CastGlobal | undefined = (globalThis as { cast?: CastGlobal }).cast) {
  const framework = cast?.framework;
  if (!framework) return;
  const options = new framework.CastReceiverOptions();
  options.skipPlayersLoad = true;
  options.disableIdleTimeout = true;
  options.maxInactivity = 600;
  const json = framework.system?.MessageType?.JSON ?? "JSON";
  options.customNamespaces = { [castNamespace]: json };
  const context = framework.CastReceiverContext.getInstance();
  context.addCustomMessageListener(castNamespace, onCastMessage);
  context.start(options);
}

window.addEventListener("pagehide", () => {
  session?.stop();
});

bootCastReceiver();
