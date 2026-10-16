import { useState } from "react";
import { useHasFrame } from "./frames";

/**
 * A preview the server already has. Not requested until it is listed and near
 * the screen, and invisible until it loads. An image with the hidden attribute
 * never loads lazily, so the wait is a class.
 */
export function LiveFrame({ id, width = 480, className }: { id: number; width?: 480 | 1280; className?: string }) {
  const listed = useHasFrame(id);
  const [state, setState] = useState<"wait" | "on" | "gone">("wait");
  if (!listed || state === "gone") return null;
  return (
    <img
      className={state === "on" ? className : `${className ?? ""} lf-wait`}
      alt=""
      loading="lazy"
      decoding="async"
      src={`/api/v1/channels/${id}/frame?w=${width}`}
      onLoad={() => setState("on")}
      onError={() => setState("gone")}
    />
  );
}
