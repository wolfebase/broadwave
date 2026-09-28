import { recordingDownloadURL } from "./download";

/** Plain link, so a multi-gigabyte file is saved by the browser and never read into the page. */
export function DownloadLink({ id, status }: { id: number; status: string }) {
  const href = recordingDownloadURL(id, status);
  if (!href) return null;
  return (
    <a className="btn" href={href} download>
      Download
    </a>
  );
}
