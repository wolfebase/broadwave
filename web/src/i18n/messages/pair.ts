import type { Message } from "../runtime.ts";

export const pairMessages = {
  "pair.codeLabel": "Pairing code",
  "pair.inviteLabel": "Invite code",
  "pair.codeHelp": "Six digits from Settings on a signed-in device.",
  "pair.inviteHelp": "The invite code from the person who runs this server.",
  "pair.useInvite": "I have an invite code",
  "pair.useDigits": "I have a 6-digit code",
  "pair.thisDevice": "This device",
  "pair.nameLabel": "Name",
  "pair.nameHelp": "How it shows up in Settings.",
  "pair.kindLabel": "Kind of device",
  "pair.kindPhone": "Phone or tablet",
  "pair.kindComputer": "Computer",
  "pair.kindTv": "TV",
  "pair.pairing": "Pairing…",
  "pair.or": "No code?",
  "pair.lockedTitle": "Locked out of your own server?",
  "pair.lockedLede":
    "If no signed-in device is left, restart the server once with the setting BROADWAVE_RESET_SIGNIN=1 (in Docker or the Unraid template). It turns sign-in off, so this browser can open Settings and pair. Then remove the setting.",
  "pair.lockedLink": "Step by step",
  "pair.orLede": "Show a code here, then approve it from a device that is already signed in.",
  "pair.waiting": "Waiting for approval…",
  "pair.yourCode": "Your code",
  "pair.newCode": "Show a new code",
} as const satisfies Record<string, Message>;
