// Request signing. A server signs every call with the Ed25519 key it
// registered, so only that server can change its own name.
//
// message = "broadwave-direct-v1\n" + method + "\n" + path + "\n" + time + "\n" + body

export const skewSeconds = 300;

export function signedMessage(method: string, path: string, time: string, body: string): Uint8Array {
  return new TextEncoder().encode(`broadwave-direct-v1\n${method.toUpperCase()}\n${path}\n${time}\n${body}`);
}

export function decodeBase64(s: string): Uint8Array | null {
  try {
    const bin = atob(s);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  } catch {
    return null;
  }
}

export function encodeBase64(b: Uint8Array): string {
  let s = "";
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s);
}

export async function verify(publicKey: string, signature: string, message: Uint8Array): Promise<boolean> {
  const pk = decodeBase64(publicKey);
  const sig = decodeBase64(signature);
  if (!pk || pk.length !== 32 || !sig || sig.length !== 64) return false;
  try {
    const key = await crypto.subtle.importKey("raw", pk, { name: "Ed25519" }, false, ["verify"]);
    return await crypto.subtle.verify({ name: "Ed25519" }, key, sig, message);
  } catch {
    return false;
  }
}

// freshTime reports a time header within the allowed clock skew.
export function freshTime(raw: string | null, nowSeconds: number): boolean {
  if (!raw || !/^[0-9]{1,12}$/.test(raw)) return false;
  return Math.abs(Number(raw) - nowSeconds) <= skewSeconds;
}
