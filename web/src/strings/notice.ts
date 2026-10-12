import { noticeMessages } from "../i18n/messages/notice.ts";
import { bind } from "../i18n/runtime.ts";

// The shell shows this before any page chunk loads. The rest of the copy table
// stays with the pages that use it, so it is not in the first script.
const t = bind(noticeMessages);

export function encryptedBroadcast(): string {
  return t("notice.encrypted");
}
