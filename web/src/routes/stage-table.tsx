import { ArrowDown, ArrowUp, Plus, X } from "lucide-react";
import { useEffect, useState } from "react";
import {
  useConfirmStageDraft,
  useDiscardStageDraft,
  useSaveStageDraft,
  useStageOverview,
  useStartStageDraft,
} from "@/api/stages";
import type { Stage, StageMap, StageOverview } from "@/api/types.legacy";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDate, formatNumber } from "@/lib/format";

const statusLabel = (s: string) => (s === "" ? "No status" : s);
const NONE = "__none__";

// The status-to-stage table. Admins edit a draft and confirm it as a new
// version; until a version is confirmed, no lead has a stage.
export function StageTable() {
  const overview = useStageOverview();
  const session = useSession();
  if (overview.isPending) return <LoadingRows rows={8} />;
  if (overview.error) return <ErrorNote error={overview.error} />;
  const o = overview.data;
  const admin = session.role === "admin";
  return (
    <div className="space-y-6">
      <VersionLine overview={o} />
      {o.draft && admin ? (
        <DraftEditor overview={o} draft={o.draft} />
      ) : (
        <CurrentTable overview={o} admin={admin} />
      )}
    </div>
  );
}

function VersionLine({ overview }: { overview: StageOverview }) {
  const c = overview.current;
  return (
    <p className="flex flex-wrap items-center gap-2 text-sm">
      {c?.version ? (
        <>
          <StatusMark status="ready" />
          Version {c.version}, confirmed{" "}
          {c.confirmedAt &&
            formatDate(c.confirmedAt, { day: "numeric", month: "short", year: "numeric" })}
          .
          {overview.history.length > 1 &&
            ` ${overview.history.length - 1} earlier ${overview.history.length === 2 ? "version is" : "versions are"} kept for saved reports.`}
        </>
      ) : (
        <>
          <StatusMark status="caution" />
          No table is confirmed yet, so funnel figures show Not available. Who at Reportage confirms
          it is still open.
        </>
      )}
    </p>
  );
}

function CurrentTable({ overview, admin }: { overview: StageOverview; admin: boolean }) {
  const start = useStartStageDraft();
  const c = overview.current;
  const stages = c?.stages ?? [];
  const flags = new Map(stages.map((s) => [s.name, s]));
  return (
    <Panel
      title="GoHighLevel status to Adx stage"
      description={
        c ? "The confirmed version. Edit it as a new draft to change anything." : undefined
      }
      action={
        admin && (
          <Button onClick={() => start.mutate()} disabled={start.isPending}>
            {c ? "Edit as a new draft" : "Start the table"}
          </Button>
        )
      }
      bodyClassName="p-0"
    >
      {start.error && (
        <div className="p-5">
          <ErrorNote error={start.error} />
        </div>
      )}
      {overview.statuses.length === 0 ? (
        <EmptyState title="No statuses yet">
          They appear once leads are imported from GoHighLevel.
        </EmptyState>
      ) : (
        <Table>
          <THead>
            <TR>
              <TH>Status in GoHighLevel</TH>
              <TH numeric>Leads</TH>
              <TH>Adx stage</TH>
              <TH>Counts as</TH>
            </TR>
          </THead>
          <TBody>
            {overview.statuses.map((st) => {
              const stage = st.currentStage ? flags.get(st.currentStage) : undefined;
              return (
                <TR key={st.status}>
                  <TD className="font-medium">{statusLabel(st.status)}</TD>
                  <TD numeric>{formatNumber(st.leads)}</TD>
                  <TD>{st.currentStage ?? <StatusChip status="caution">No stage</StatusChip>}</TD>
                  <TD className="text-sm text-muted-foreground">{qualified(stage)}</TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      )}
    </Panel>
  );
}

function qualified(s: Stage | undefined) {
  if (!s) return "";
  return [s.mql && "MQL", s.sql && "SQL"].filter(Boolean).join(", ") || "Neither";
}

function DraftEditor({ overview, draft }: { overview: StageOverview; draft: StageMap }) {
  const save = useSaveStageDraft();
  const confirm = useConfirmStageDraft();
  const discard = useDiscardStageDraft();
  const [stages, setStages] = useState<Stage[]>(draft.stages);
  const [rules, setRules] = useState<Record<string, string>>(() =>
    Object.fromEntries(draft.rules.map((r) => [r.status, r.stage])),
  );
  const [dirty, setDirty] = useState(false);
  useEffect(() => {
    setStages(draft.stages);
    setRules(Object.fromEntries(draft.rules.map((r) => [r.status, r.stage])));
    setDirty(false);
  }, [draft]);

  const names = stages.map((s) => s.name.trim()).filter(Boolean);
  const unmapped = overview.statuses.filter(
    (st) => !rules[st.status] || !names.includes(rules[st.status] ?? ""),
  );
  const nextVersion = (overview.current?.version ?? 0) + 1;
  const busy = save.isPending || confirm.isPending || discard.isPending;
  const error = save.error ?? confirm.error ?? discard.error;
  const payload = () => ({
    stages,
    rules: Object.entries(rules)
      .filter(([, stage]) => names.includes(stage))
      .map(([status, stage]) => ({ status, stage })),
  });
  const editStage = (i: number, patch: Partial<Stage>) => {
    setStages((list) => list.map((s, j) => (j === i ? { ...s, ...patch } : s)));
    if (patch.name !== undefined) {
      const old = stages[i]?.name;
      setRules((r) =>
        Object.fromEntries(
          Object.entries(r).map(([k, v]) => [k, v === old ? (patch.name ?? v) : v]),
        ),
      );
    }
    setDirty(true);
  };
  const move = (i: number, d: -1 | 1) => {
    setStages((list) => {
      const j = i + d;
      if (j < 0 || j >= list.length) return list;
      const next = [...list];
      [next[i], next[j]] = [next[j] as Stage, next[i] as Stage];
      return next;
    });
    setDirty(true);
  };

  return (
    <>
      <div className="flex flex-wrap items-center gap-3 border px-4 py-3">
        <span className="text-sm">
          Draft of version {nextVersion}. Nothing changes until it's confirmed.
          {unmapped.length > 0 &&
            ` ${unmapped.length} ${unmapped.length === 1 ? "status has" : "statuses have"} no stage yet.`}
        </span>
        <span className="ml-auto flex flex-wrap gap-2">
          <Button variant="ghost" disabled={busy} onClick={() => discard.mutate()}>
            Discard the draft
          </Button>
          <Button
            variant="outline"
            disabled={busy || !dirty}
            onClick={() => save.mutate(payload(), { onSuccess: () => setDirty(false) })}
          >
            {save.isPending ? "Saving…" : "Save the draft"}
          </Button>
          <Button
            disabled={busy}
            onClick={() =>
              save.mutate(payload(), {
                onSuccess: () => {
                  setDirty(false);
                  confirm.mutate();
                },
              })
            }
          >
            Confirm as version {nextVersion}
          </Button>
        </span>
      </div>
      {error && <ErrorNote error={error} />}

      <div className="grid gap-6 xl:grid-cols-[22rem_1fr]">
        <Panel
          title="Stages"
          description="In funnel order. Tick what counts as marketing-qualified (MQL) and sales-qualified (SQL)."
          bodyClassName="p-0"
        >
          <ul className="divide-y">
            {stages.map((s, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: names change while typing, so position is the identity
              <li key={i} className="flex items-center gap-2 px-4 py-2">
                <Input
                  aria-label={`Stage ${i + 1}`}
                  value={s.name}
                  onChange={(e) => editStage(i, { name: e.target.value })}
                  className="h-7"
                />
                <label className="flex items-center gap-1 text-xs">
                  <input
                    type="checkbox"
                    checked={s.mql}
                    onChange={(e) => editStage(i, { mql: e.target.checked })}
                  />
                  MQL
                </label>
                <label className="flex items-center gap-1 text-xs">
                  <input
                    type="checkbox"
                    checked={s.sql}
                    onChange={(e) => editStage(i, { sql: e.target.checked })}
                  />
                  SQL
                </label>
                <span className="flex">
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    aria-label="Move up"
                    onClick={() => move(i, -1)}
                  >
                    <ArrowUp />
                  </Button>
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    aria-label="Move down"
                    onClick={() => move(i, 1)}
                  >
                    <ArrowDown />
                  </Button>
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    aria-label={`Remove ${s.name}`}
                    onClick={() => {
                      setStages((list) => list.filter((_, j) => j !== i));
                      setDirty(true);
                    }}
                  >
                    <X />
                  </Button>
                </span>
              </li>
            ))}
          </ul>
          <div className="border-t px-4 py-2">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setStages((list) => [...list, { name: "", mql: false, sql: false }]);
                setDirty(true);
              }}
            >
              <Plus />
              Stage
            </Button>
          </div>
        </Panel>

        <Panel title="GoHighLevel status to Adx stage" bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Status in GoHighLevel</TH>
                <TH numeric>Leads</TH>
                <TH>Now</TH>
                <TH className="w-56">In this draft</TH>
              </TR>
            </THead>
            <TBody>
              {overview.statuses.map((st) => {
                const value =
                  rules[st.status] && names.includes(rules[st.status] ?? "")
                    ? (rules[st.status] ?? NONE)
                    : NONE;
                return (
                  <TR key={st.status}>
                    <TD className="font-medium">{statusLabel(st.status)}</TD>
                    <TD numeric>{formatNumber(st.leads)}</TD>
                    <TD className="text-sm text-muted-foreground">
                      {st.currentStage ?? "No stage"}
                    </TD>
                    <TD>
                      <Select
                        aria-label={`Stage for ${statusLabel(st.status)}`}
                        value={value}
                        onValueChange={(v) => {
                          setRules((r) => {
                            const next = { ...r };
                            if (v === NONE) delete next[st.status];
                            else next[st.status] = v;
                            return next;
                          });
                          setDirty(true);
                        }}
                        options={[
                          { value: NONE, label: "No stage", icon: <StatusMark status="caution" /> },
                          ...names.map((n) => ({ value: n, label: n })),
                        ]}
                        className="w-full"
                      />
                    </TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </Panel>
      </div>
    </>
  );
}
