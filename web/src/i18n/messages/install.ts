import type { Message } from "../runtime.ts";

export const installMessages = {
  "install.region": "Install Broadwave",
  "install.lede": "Install Broadwave on this device. It opens like an app.",
  "install.install": "Install",
  "install.notNow": "Not now",
  "install.iosHint": "Add Broadwave to your Home Screen: tap Share, then Add to Home Screen. It opens like an app.",
  "install.gotIt": "Got it",
  "install.failed": "Couldn't open the install dialog. Try again from the browser menu.",
} as const satisfies Record<string, Message>;
