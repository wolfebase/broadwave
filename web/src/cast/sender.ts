// The sender SDK is loaded from Google the first time someone presses Cast.
// A test can install window.cast.framework first; then this does not add a script.

import { castNamespace, senderSDK } from "./protocol.ts";

export { senderSDK };

export type CastLink = {
  send(data: unknown): void;
  end(): void;
};

type CastSessionLike = {
  sendMessage(namespace: string, data: unknown): Promise<unknown>;
  endSession(stopCasting: boolean): void;
};

type SessionStateEvent = { sessionState?: string };

type CastContextLike = {
  setOptions(options: { receiverApplicationId: string; autoJoinPolicy: string }): void;
  requestSession(): Promise<unknown>;
  getCurrentSession(): CastSessionLike | null;
  addEventListener?(type: string, listener: (event: SessionStateEvent) => void): void;
};

type SenderScript = { src: string; onerror: (() => void) | null };

export type SenderEnv = {
  cast?: {
    framework?: {
      CastContext?: { getInstance(): CastContextLike };
      CastContextEventType?: { SESSION_STATE_CHANGED?: string };
      SessionState?: { SESSION_ENDED?: string };
    };
  };
  chrome?: { cast?: { AutoJoinPolicy?: { ORIGIN_SCOPED?: string } } };
  document?: { createElement(tag: string): SenderScript; head: { appendChild(node: SenderScript): void } };
  window?: { __onGCastApiAvailable?: (available: boolean) => void };
};

function browserSenderEnv(): SenderEnv {
  const host = window as Window & { cast?: SenderEnv["cast"]; chrome?: SenderEnv["chrome"]; __onGCastApiAvailable?: (available: boolean) => void };
  return { cast: host.cast, chrome: host.chrome, document: document as unknown as SenderEnv["document"], window: host };
}

/** True when the sender framework is ready. False when the script fails or times out. */
export function ensureSender(env: SenderEnv = browserSenderEnv(), timeoutMs = 8000): Promise<boolean> {
  if (env.cast?.framework) return Promise.resolve(true);
  const documentHost = env.document;
  const host = env.window;
  if (!documentHost || !host) return Promise.resolve(false);
  return new Promise((resolve) => {
    let settled = false;
    const timer = setTimeout(() => finish(false), timeoutMs);
    function finish(ok: boolean) {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(ok);
    }
    host.__onGCastApiAvailable = (available) => finish(!!available);
    const script = documentHost.createElement("script");
    script.src = senderSDK;
    script.onerror = () => finish(false);
    documentHost.head.appendChild(script);
  });
}

// The context is one per page. Its end listener is added once and calls the latest button.
const listening = new WeakSet<CastContextLike>();
let ending: (() => void) | undefined;

/**
 * Opens the device picker, or the session already on this page. Null when the
 * picker is closed without a choice. Throws when the SDK is missing or no device answers.
 */
export async function requestCast(appId: string, env: SenderEnv = browserSenderEnv(), onEnd?: () => void): Promise<CastLink | null> {
  const framework = env.cast?.framework;
  const Context = framework?.CastContext;
  if (!Context) throw new Error("Cast didn't load.");
  const context = Context.getInstance();
  const policy = env.chrome?.cast?.AutoJoinPolicy?.ORIGIN_SCOPED ?? "origin_scoped";
  context.setOptions({ receiverApplicationId: appId, autoJoinPolicy: policy });
  // setOptions does not start a session. One left from a previous visit is reused
  // so the picker does not open over a television that is already playing.
  let session = context.getCurrentSession();
  if (!session) {
    try {
      await context.requestSession();
    } catch (err) {
      // The sender SDK rejects with chrome.cast.ErrorCode.CANCEL when the picker is dismissed.
      if (err === "cancel") return null;
      throw new Error("No Cast devices found.", { cause: err });
    }
    session = context.getCurrentSession();
  }
  if (!session) throw new Error("No Cast devices found.");
  const eventType = framework?.CastContextEventType?.SESSION_STATE_CHANGED ?? "sessionstatechanged";
  const ended = framework?.SessionState?.SESSION_ENDED ?? "sessionended";
  ending = onEnd;
  if (context.addEventListener && !listening.has(context)) {
    listening.add(context);
    context.addEventListener(eventType, (event) => {
      if (event.sessionState === ended) ending?.();
    });
  }
  return {
    send(data) {
      void session.sendMessage(castNamespace, data);
    },
    end() {
      session.endSession(true);
    },
  };
}
