/** A finished recording is a file the browser saves. One still being written has no link. */
export function recordingDownloadURL(id: number, status: string): string | null {
  if (status === "recording") return null;
  return `/api/v1/recordings/${id}/file`;
}
