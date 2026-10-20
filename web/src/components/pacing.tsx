import type { Pacing } from "@/api/models";
import { Meter } from "@/components/charts";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { dubaiToday } from "@/lib/analytics";
import { formatDate, formatMoneyShort, formatPercent } from "@/lib/format";

// A month heading this far over or under its budget is worth watching, and
// this far needs action. The brief and the Campaigns screen use the same
// lines, so the colour means the same thing on both.
export const PACE = { watch: 0.05, act: 0.1 } as const;

// paceOf is where a month is heading against its budget: the share it lands
// over (positive) or under (negative), and that as a status.
export function paceOf(pacing: Pacing): { off: number; status: Status } {
  const off = pacing.projectedMinor / pacing.budgetMinor - 1;
  const size = Math.abs(off);
  return { off, status: size < PACE.watch ? "ready" : size < PACE.act ? "caution" : "error" };
}

// PacingCell is a project's month: spent against budget, where it should be
// by today, and whether it lands over or under by month end.
export function PacingCell({ pacing }: { pacing: Pacing | null }) {
  if (!pacing) return <p className="text-xs text-muted-foreground">No monthly budget set</p>;
  const expected = (pacing.budgetMinor * pacing.dayOfMonth) / pacing.daysInMonth;
  const { off, status } = paceOf(pacing);
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
      <Meter value={pacing.spentMinor} max={pacing.budgetMinor} marker={expected} status={status} />
      <p className="flex items-center gap-1.5 text-xs whitespace-nowrap">
        <StatusMark status={status} />
        {status === "ready"
          ? "On track"
          : `${formatPercent(Math.abs(off))} ${off > 0 ? "over" : "under"} by month end`}
      </p>
    </div>
  );
}
