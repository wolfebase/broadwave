// This web build. Compared with the server's minAppVersion, not the server's own version.
export const appVersion = "1.0";

export const updateServer = "Update your Broadwave server to use this";
export const updateApp = "Update Broadwave to use this server";

type ServerCaps = {
  apiVersion?: number;
  version?: string;
  features?: readonly string[];
  minAppVersion?: string;
};

// Missing numeric pieces count as 0, so 1.0 and 1.0.0 are the same app.
export function compareVersions(left: string, right: string): number {
  const a = parts(left);
  const b = parts(right);
  const n = Math.max(a.length, b.length);
  for (let i = 0; i < n; i++) {
    const da = a[i] ?? 0;
    const db = b[i] ?? 0;
    if (da !== db) return da < db ? -1 : 1;
  }
  return 0;
}

function parts(version: string): number[] {
  return version.split(".").map((piece) => {
    const digits = /^(\d+)/.exec(piece.trim());
    return digits ? Number(digits[1]) : 0;
  });
}

// The sentence to show in place of a control, or null when the server can do it.
// No features list means a server from before negotiation: leave the control up.
export function gateFeature(server: ServerCaps | null | undefined, feature: string): string | null {
  if (!server?.features) return null;
  return server.features.includes(feature) ? null : updateServer;
}

// Full-screen block when this app is older than the server asks for.
// A newer apiVersion alone does not block. The server's version string is not minAppVersion.
export function gateApp(server: ServerCaps | null | undefined, app = appVersion): string | null {
  const need = server?.minAppVersion?.trim();
  if (!need) return null;
  return compareVersions(app, need) < 0 ? updateApp : null;
}
