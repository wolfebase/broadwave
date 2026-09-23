import { useState } from "react";

/** A preview from a mux that is already on. Hidden until one exists. */
export function LiveFrame({ id, width = 480, className }: { id: number; width?: 480 | 1280; className?: string }) {
  const [on, setOn] = useState(false);
  return <img className={className} alt="" hidden={!on} src={`/api/v1/channels/${id}/frame?w=${width}`} onLoad={() => setOn(true)} onError={() => setOn(false)} />;
}
