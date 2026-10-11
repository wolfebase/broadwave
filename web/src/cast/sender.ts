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

type CastContextLike = {
  setOptions(options: { receiverApplicationId: string; autoJoinPolicy: string }): void;
  requestSession(): Promise<unknown>;
  getCurrentSession(): CastSessionLike | null;
};

type SenderScript = { src: string; onerror: (() => void) | null };

export type SenderEnv = {
  cast?: { framework?: { CastContext?: { getInstance(): CastContextLike } } };
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

/** Opens the device picker. Throws when the SDK is missing or no device is chosen. */
export async function requestCast(appId: string, env: SenderEnv = browserSenderEnv()): Promise<CastLink> {
  const Context = env.cast?.framework?.CastContext;
  if (!Context) throw new Error("Cast didn't load.");
  const context = Context.getInstance();
  const policy = env.chrome?.cast?.AutoJoinPolicy?.ORIGIN_SCOPED ?? "origin_scoped";
  context.setOptions({ receiverApplicationId: appId, autoJoinPolicy: policy });
  try {
    await context.requestSession();
  } catch {
    throw new Error("No Cast devices found.");
  }
  const session = context.getCurrentSession();
  if (!session) throw new Error("No Cast devices found.");
  return {
    send(data) {
      void session.sendMessage(castNamespace, data);
    },
    end() {
      session.endSession(true);
    },
  };
}
