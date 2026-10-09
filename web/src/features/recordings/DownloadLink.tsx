import { actionName } from "../../lib/actionName";
import { recordingDownloadURL } from "./download";

/** Plain link, so a multi-gigabyte file is saved by the browser and never read into the page. */
export function DownloadLink({ id, status, name }: { id: number; status: string; name?: string }) {
  const href = recordingDownloadURL(id, status);
  if (!href) return null;
  return (
    <a className="btn" href={href} download aria-label={name ? actionName("Download", name) : undefined}>
      Download
    </a>
  );
}
