import { useState } from "react";
import { useHasFrame } from "./frames";

/** A preview the server already has. Not requested until it is listed, and hidden until it loads. */
export function LiveFrame({ id, width = 480, className }: { id: number; width?: 480 | 1280; className?: string }) {
  const listed = useHasFrame(id);
  const [on, setOn] = useState(false);
  if (!listed) return null;
  return <img className={className} alt="" hidden={!on} src={`/api/v1/channels/${id}/frame?w=${width}`} onLoad={() => setOn(true)} onError={() => setOn(false)} />;
}
