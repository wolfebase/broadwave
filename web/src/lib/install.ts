export const installDismissKey = "broadwave.install.dismissed";

/** The banner is for a browser that asked, and only until it is installed or refused. */
export function offerInstall(state: { standalone: boolean; dismissed: boolean; hasPrompt: boolean }): boolean {
  return state.hasPrompt && !state.standalone && !state.dismissed;
}

export function readDismissed(storage: { getItem(key: string): string | null } | null): boolean {
  try {
    return storage?.getItem(installDismissKey) === "1";
  } catch {
    return false;
  }
}

/** True when the banner must not show. A private window that hides storage counts as refused. */
export function installBlocked(): boolean {
  let dismissed: boolean;
  try {
    dismissed = localStorage.getItem(installDismissKey) === "1";
  } catch {
    dismissed = true;
  }
  const ios = (navigator as Navigator & { standalone?: boolean }).standalone === true;
  const standalone = ios || window.matchMedia("(display-mode: standalone)").matches;
  return !offerInstall({ standalone, dismissed, hasPrompt: true });
}
