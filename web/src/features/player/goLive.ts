export type GoLiveAction = "group" | "arm" | "play";

/** Sync already owns the playhead. Do not seek to seekable.end - 10. */
export function goLiveAction(together: boolean, syncOn: boolean): GoLiveAction {
  if (together) return "group";
  if (!syncOn) return "arm";
  return "play";
}
