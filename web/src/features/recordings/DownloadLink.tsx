import { recordingsMessages } from "../../i18n/messages/recordings.ts";
import { bind } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { recordingDownloadURL } from "./download";

const t = bind(recordingsMessages);

/** Plain link, so a multi-gigabyte file is saved by the browser and never read into the page. */
export function DownloadLink({ id, status, name }: { id: number; status: string; name?: string }) {
  const href = recordingDownloadURL(id, status);
  if (!href) return null;
  const label = t("recordings.download");
  return (
    <a className="btn" href={href} download aria-label={name ? actionName(label, name) : undefined}>
      {label}
    </a>
  );
}
