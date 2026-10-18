import { useEffect, useState } from "react";

// CountUp runs a figure from zero to its value when it first appears, so a
// front or a Focus slide reads as alive rather than pasted. The final text is
// whatever the server formatted; only the digits move.
export function CountUp({
  value,
  to,
  duration = 900,
}: {
  value: string;
  to?: number;
  duration?: number;
}) {
  const [shown, setShown] = useState(to === undefined ? value : "0");
  useEffect(() => {
    if (to === undefined) {
      setShown(value);
      return;
    }
    if (
      typeof window === "undefined" ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      setShown(value);
      return;
    }
    const decimals = (value.split(".")[1] ?? "").replace(/\D/g, "").length;
    const grouped = value.includes(",");
    const start = performance.now();
    let frame = 0;
    const tick = (t: number) => {
      const k = Math.min(1, (t - start) / duration);
      const eased = 1 - (1 - k) ** 3;
      const n = to * eased;
      setShown(k >= 1 ? value : grouped ? Math.round(n).toLocaleString("en") : n.toFixed(decimals));
      if (k < 1) {
        frame = requestAnimationFrame(tick);
      }
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [value, to, duration]);
  return <span className="tabular-nums">{shown}</span>;
}
