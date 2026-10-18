import { useAudiences } from "@/api/adx";
import type { Audience } from "@/api/models";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { StatusChip } from "@/components/ui/status-chip";
import { formatNumber, formatPercent, formatRelative } from "@/lib/format";

export function AudiencesPage() {
  const audiences = useAudiences();
  return (
    <>
      <PageHeader title="Audiences" />
      {audiences.isPending && <LoadingRows rows={5} />}
      {audiences.error && <ErrorNote error={audiences.error} />}
      {audiences.data && (
        <div className="grid gap-4 lg:grid-cols-2">
          {audiences.data.map((a) => (
            <AudienceCard key={a.id} audience={a} />
          ))}
        </div>
      )}
    </>
  );
}

function AudienceCard({ audience: a }: { audience: Audience }) {
  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          <PlatformTag platform={a.platform} />
          <span className="font-mono text-sm break-all">{a.name}</span>
        </span>
      }
      description={a.purpose}
      action={<StatusChip status={a.state}>{a.stateLabel}</StatusChip>}
    >
      <p className="text-sm text-muted-foreground">{a.rule}</p>
      <dl className="mt-4 grid grid-cols-3 gap-4 text-sm">
        <Figure label="People" value={formatNumber(a.size)} />
        <Figure label="Matched" value={formatPercent(a.matchRate)} />
        <Figure label="Refreshed" value={formatRelative(a.refreshedAt)} />
      </dl>
      <Lift lift={a.lift} />
    </Panel>
  );
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="font-mono text-[0.65rem] tracking-wider text-muted-foreground uppercase">
        {label}
      </dt>
      <dd className="mt-0.5 font-mono tabular-nums">{value}</dd>
    </div>
  );
}

// Lift compares people who saw the ads with the held-back group. Without a
// holdout there's no way to tell the audience from leads who'd have come back anyway.
function Lift({ lift }: { lift: Audience["lift"] }) {
  if (!lift) {
    return (
      <p className="mt-4 border-t pt-3 text-xs text-muted-foreground">No holdout result yet.</p>
    );
  }
  const max = Math.max(lift.returnedRate, lift.holdoutRate, 0.0001);
  const ratio = lift.holdoutRate > 0 ? lift.returnedRate / lift.holdoutRate : null;
  return (
    <div className="mt-4 space-y-2 border-t pt-3">
      <div className="flex items-baseline justify-between text-sm">
        <span className="font-medium">Came back</span>
        <span className="text-xs text-muted-foreground">
          {ratio !== null
            ? `${ratio.toFixed(1)}× the held-back group`
            : "No one in the held-back group came back"}
        </span>
      </div>
      <LiftBar
        label={`Reached · ${formatNumber(lift.reached)}`}
        rate={lift.returnedRate}
        max={max}
        strong
      />
      <LiftBar
        label={`Held back · ${formatNumber(lift.holdout)}`}
        rate={lift.holdoutRate}
        max={max}
      />
    </div>
  );
}

function LiftBar({
  label,
  rate,
  max,
  strong,
}: {
  label: string;
  rate: number;
  max: number;
  strong?: boolean;
}) {
  return (
    <div className="grid grid-cols-[8.5rem_1fr_3.5rem] items-center gap-3 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <div className="h-2 rounded-none bg-muted">
        <div
          className={
            strong ? "h-full rounded-none bg-foreground" : "h-full rounded-none bg-foreground/35"
          }
          style={{ width: `${(rate / max) * 100}%` }}
        />
      </div>
      <span className="text-right font-mono tabular-nums">{formatPercent(rate, 1)}</span>
    </div>
  );
}
