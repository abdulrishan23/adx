import type { Pacing } from "@/api/models";
import { Meter } from "@/components/charts";
import { StatusMark } from "@/components/ui/status-chip";
import { dubaiToday } from "@/lib/analytics";
import { formatDate, formatMoneyShort, formatPercent } from "@/lib/format";

// PacingCell is a project's month: spent against budget, where it should be
// by today, and whether it lands over or under by month end.
export function PacingCell({ pacing }: { pacing: Pacing | null }) {
  if (!pacing) return <p className="text-xs text-muted-foreground">No monthly budget set</p>;
  const expected = (pacing.budgetMinor * pacing.dayOfMonth) / pacing.daysInMonth;
  const off = pacing.projectedMinor / pacing.budgetMinor - 1;
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-3 text-xs">
        <span className="text-muted-foreground">
          {formatDate(`${dubaiToday()}T00:00:00Z`, { month: "long" })} budget
        </span>
        <span className="font-mono tabular-nums">
          {formatMoneyShort(pacing.spentMinor)} of {formatMoneyShort(pacing.budgetMinor)}
        </span>
      </div>
      <Meter value={pacing.spentMinor} max={pacing.budgetMinor} marker={expected} />
      <p className="text-xs whitespace-nowrap">
        {Math.abs(off) < 0.05 ? (
          <span className="text-muted-foreground">On track</span>
        ) : (
          <>
            <StatusMark status={off > 0 ? "caution" : "pending"} className="mr-1.5" />
            {formatPercent(Math.abs(off))} {off > 0 ? "over" : "under"} by month end
          </>
        )}
      </p>
    </div>
  );
}
