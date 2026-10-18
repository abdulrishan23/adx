import type { Temperature } from "@/api/models";
import { cn } from "@/lib/utils";

const LEVEL: Record<Temperature, number> = { cold: 1, warm: 2, hot: 3 };
const LABEL: Record<Temperature, string> = { cold: "Cold", warm: "Warm", hot: "Hot" };

// TemperatureMeter is a three-segment meter, not a coloured pill, so it reads
// the same in both themes and stays inside the monochrome look.
export function TemperatureMeter({
  temperature,
  showLabel = true,
  className,
}: {
  temperature: Temperature;
  showLabel?: boolean;
  className?: string;
}) {
  const level = LEVEL[temperature];
  return (
    <span className={cn("inline-flex items-center gap-2", className)} title={LABEL[temperature]}>
      <span className="flex gap-0.5" aria-hidden="true">
        {[1, 2, 3].map((i) => (
          <span
            key={i}
            className={cn(
              "h-3 w-1.5 rounded-none",
              i <= level ? "bg-foreground" : "bg-foreground/15",
            )}
          />
        ))}
      </span>
      {showLabel && <span className="text-sm">{LABEL[temperature]}</span>}
      {!showLabel && <span className="sr-only">{LABEL[temperature]}</span>}
    </span>
  );
}
