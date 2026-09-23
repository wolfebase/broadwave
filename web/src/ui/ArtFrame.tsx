import { useEffect, useRef, useState } from "react";
import { artLayout, displayEdge } from "../lib/art";
import "./art.css";

export function ArtFrame({ src, width = 0, height = 0 }: { src: string; width?: number; height?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const [slot, setSlot] = useState(0);
  const [hidden, setHidden] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => setSlot(el.clientWidth);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  if (hidden) return null;
  const layout = artLayout(width, height, slot);
  const edge = displayEdge(Math.max(width, height), slot);
  return (
    <div ref={ref} className={`art-stage ${layout}`} data-layout={layout} style={edge > 0 ? { ["--art-edge" as string]: `${edge}px` } : undefined}>
      <img className="art-blur" alt="" src={src} />
      <img className="art-sharp" alt="" src={src} onError={() => setHidden(true)} />
    </div>
  );
}
