import { useEffect, useRef } from "react";
import type { DesignDoc } from "./doc";
import { buildStage } from "./render";

// A small live picture of a design file, drawn off the editor's canvas.
export function DocPreview({
  doc,
  width,
  className,
}: {
  doc: DesignDoc;
  width: number;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const scale = width / doc.width;
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let stage: Awaited<ReturnType<typeof buildStage>> | null = null;
    let cancelled = false;
    void buildStage(doc, scale, el).then((s) => {
      if (cancelled) s.destroy();
      else stage = s;
    });
    return () => {
      cancelled = true;
      stage?.destroy();
      el.innerHTML = "";
    };
  }, [doc, scale]);
  return (
    <div
      ref={ref}
      className={className}
      style={{ width, height: Math.round(doc.height * scale) }}
    />
  );
}
