import { type FormEvent, useId, useState } from "react";
import { useGuardrails, useUpdateGuardrails } from "@/api/adx";
import type { AccountGuard, Guardrails } from "@/api/models";
import { Meter } from "@/components/charts";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatMoneyShort, formatRelative } from "@/lib/format";
import { released } from "@/lib/release";

// The write actions D-13 lists, which arrive with proposals in phase 3.
const WRITES = [
  "Create or update audiences and Customer Match lists",
  "Upload offline conversions and Conversions API events",
  "Exclusion lists, labels and negative keywords",
  "Pause an ad, ad set or campaign",
  "Change budgets, bids or targets",
  "Create or launch campaigns and ads",
  "Write lead fields in GoHighLevel",
];

export function GuardrailsPage() {
  if (!released(3)) return <NothingWrites />;
  return <ConfiguredGuardrails />;
}

// Before proposals ship, Adx only reads. The page says so plainly instead of
// showing caps that nothing uses.
function NothingWrites() {
  return (
    <>
      <PageHeader
        title="Guardrails"
        subtitle="Limits on what Adx may change in Google Ads, Meta and GoHighLevel."
      />
      <div className="space-y-6">
        <p className="flex items-center gap-2 text-sm">
          <StatusMark status="ready" />
          Adx only reads in this release. It changes nothing in Google Ads, Meta or GoHighLevel, so
          there are no limits to set yet.
        </p>
        <Panel
          title="Write actions"
          description="Each one stays off until proposals arrive and an admin sets its limits and who approves it."
          bodyClassName="p-0"
        >
          <ul className="divide-y">
            {WRITES.map((w) => (
              <li key={w} className="flex items-center justify-between gap-4 px-5 py-3 text-sm">
                {w}
                <StatusChip status="pending">Off</StatusChip>
              </li>
            ))}
          </ul>
        </Panel>
        <Panel title="Still to agree with Reportage">
          <ul className="list-disc space-y-1.5 pl-5 text-sm text-muted-foreground">
            <li>Monthly budgets per account, and the largest daily increase anyone may make.</li>
            <li>Who approves which change, and which changes marketers may make alone.</li>
            <li>When an anomaly is urgent enough to alert outside the morning check.</li>
          </ul>
        </Panel>
      </div>
    </>
  );
}

function ConfiguredGuardrails() {
  const guard = useGuardrails();
  return (
    <>
      <PageHeader title="Guardrails" />
      {guard.isPending && <LoadingRows rows={6} />}
      {guard.error && <ErrorNote error={guard.error} />}
      {guard.data && <Body g={guard.data} />}
    </>
  );
}

function Body({ g }: { g: Guardrails }) {
  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-3">
        <LimitPanel
          title="Max daily budget increase"
          value={g.dailyIncreaseCapPct}
          suffix="%"
          field="dailyIncreaseCapPct"
        />
        <LimitPanel title="Changes per day" value={g.changesPerDay} field="changesPerDay">
          <div className="mt-4">
            <Meter value={g.changesToday} max={g.changesPerDay} />
            <p className="mt-1.5 text-xs text-muted-foreground">
              {g.changesToday} used today, {Math.max(0, g.changesPerDay - g.changesToday)} left
            </p>
          </div>
        </LimitPanel>
        <Panel title="Proposals expire">
          <p className="font-heading text-3xl font-semibold tabular-nums">{g.proposalHours}h</p>
        </Panel>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {g.accounts.map((a) => (
          <AccountPanel key={a.key} account={a} />
        ))}
      </div>

      <Panel title="Blocked" bodyClassName="p-0">
        {g.blocked.length === 0 ? (
          <p className="px-5 py-4 text-sm text-muted-foreground">
            Nothing blocked in the last 30 days.
          </p>
        ) : (
          <ul className="divide-y">
            {g.blocked.map((b) => (
              <li key={b.id} className="grid gap-1 px-5 py-3 text-sm sm:grid-cols-[1fr_auto]">
                <div>
                  <p className="font-medium">{b.action}</p>
                  <p className="text-muted-foreground">{b.reason}</p>
                </div>
                <p className="text-xs text-muted-foreground sm:text-right">
                  {b.by}
                  <br />
                  {formatRelative(b.at)}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Panel>
    </div>
  );
}

function LimitPanel({
  title,
  description,
  value,
  suffix,
  field,
  children,
}: {
  title: string;
  description?: string;
  value: number;
  suffix?: string;
  field: "dailyIncreaseCapPct" | "changesPerDay";
  children?: React.ReactNode;
}) {
  const update = useUpdateGuardrails();
  const [draft, setDraft] = useState(String(value));
  const id = useId();
  const changed = draft !== String(value);
  function onSubmit(e: FormEvent) {
    e.preventDefault();
    update.mutate({ [field]: Number(draft) });
  }
  return (
    <Panel title={title} description={description}>
      <form onSubmit={onSubmit} className="flex items-center gap-2">
        <label htmlFor={id} className="sr-only">
          {title}
        </label>
        <Input
          id={id}
          inputMode="numeric"
          value={draft}
          onChange={(e) => {
            update.reset();
            setDraft(e.target.value.replace(/\D/g, ""));
          }}
          className="h-10 w-24 font-heading text-2xl font-semibold"
        />
        {suffix && <span className="font-heading text-2xl font-semibold">{suffix}</span>}
        {changed && (
          <Button
            type="submit"
            size="sm"
            className="ml-auto"
            disabled={update.isPending || draft === ""}
          >
            Save
          </Button>
        )}
      </form>
      {update.error && (
        <div className="mt-3">
          <ErrorNote error={update.error} />
        </div>
      )}
      {children}
    </Panel>
  );
}

function AccountPanel({ account: a }: { account: AccountGuard }) {
  const update = useUpdateGuardrails();
  const [confirming, setConfirming] = useState(false);
  const [budget, setBudget] = useState(
    a.monthlyBudgetMinor === null ? "" : String(a.monthlyBudgetMinor / 100),
  );
  const budgetId = useId();
  const budgetChanged =
    budget !== (a.monthlyBudgetMinor === null ? "" : String(a.monthlyBudgetMinor / 100));

  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          <PlatformTag platform={a.platform} />
          {a.name}
        </span>
      }
      action={
        a.killed ? (
          <StatusChip status="error">Stopped</StatusChip>
        ) : (
          <StatusChip status="ready">Adx can change</StatusChip>
        )
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate({
            account: {
              key: a.key,
              monthlyBudgetMinor: budget === "" ? null : Math.round(Number(budget) * 100),
            },
          });
        }}
        className="flex flex-wrap items-end gap-3"
      >
        <div className="space-y-1">
          <label htmlFor={budgetId} className="text-xs text-muted-foreground">
            Monthly budget, AED
          </label>
          <Input
            id={budgetId}
            inputMode="numeric"
            value={budget}
            onChange={(e) => {
              update.reset();
              setBudget(e.target.value.replace(/[^\d.]/g, ""));
            }}
            className="h-9 w-40 font-mono"
          />
        </div>
        <p className="pb-2 text-xs text-muted-foreground">
          Now {formatMoneyShort(a.monthlyBudgetMinor)}
        </p>
        {budgetChanged && (
          <Button type="submit" size="sm" disabled={update.isPending}>
            Save
          </Button>
        )}
      </form>

      <div className="mt-5 border-t pt-4">
        <p className="text-sm font-medium">Kill switch</p>
        <p className="mt-1 text-sm text-muted-foreground">
          {a.killed
            ? `Stopped by ${a.killedBy}${a.killedAt ? ` ${formatRelative(a.killedAt)}` : ""}`
            : "Blocks every change, approved or not. Reporting continues."}
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          {a.killed ? (
            <Button
              variant="outline"
              size="sm"
              disabled={update.isPending}
              onClick={() => update.mutate({ account: { key: a.key, killed: false } })}
            >
              Let Adx make changes again
            </Button>
          ) : confirming ? (
            <>
              <Button
                variant="destructive"
                size="sm"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate(
                    { account: { key: a.key, killed: true } },
                    { onSettled: () => setConfirming(false) },
                  )
                }
              >
                Stop all changes now
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setConfirming(false)}>
                Cancel
              </Button>
            </>
          ) : (
            <Button variant="outline" size="sm" onClick={() => setConfirming(true)}>
              Stop all changes
            </Button>
          )}
        </div>
      </div>
      {update.error && (
        <div className="mt-3">
          <ErrorNote error={update.error} />
        </div>
      )}
    </Panel>
  );
}
