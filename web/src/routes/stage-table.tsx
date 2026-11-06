import { Plus } from "lucide-react";
import { useEffect, useState } from "react";
import { useFunnelSettings, useSaveFunnel } from "@/api/stages";
import type { FieldLocation, FunnelDocument, FunnelState, Stage } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDate, formatNumber } from "@/lib/format";
import { STAGE_LABEL } from "@/lib/leads";
import {
  blankCount,
  countsAs,
  draftOf,
  type StatusRow,
  statusRows,
  unmappedIn,
  withStage,
} from "@/lib/stages";

const NONE = "__none__";

const FIELD_KINDS: { value: string; label: string }[] = [
  { value: "", label: "Not configured" },
  { value: "custom_field", label: "Custom field (by id)" },
  { value: "contact_field", label: "Contact field (source, country)" },
  { value: "tag_prefix", label: "Tag prefix" },
];

// The status-to-stage table (core/leads: the funnel settings). Every raw
// status seen in the data is a row; an admin gives each a stage and saves
// the table as the next version, which re-stages every lead at once. A
// status without a row is flagged, never hidden. Which GoHighLevel field
// holds the status is a setting here, not a guess in code.
export function StageTable() {
  const settings = useFunnelSettings();
  const session = useSession();
  if (settings.isPending) return <LoadingRows rows={8} />;
  if (settings.error) return <ErrorNote error={settings.error} />;
  return <Editor state={settings.data} admin={session.role === "admin"} />;
}

function Editor({ state, admin }: { state: FunnelState; admin: boolean }) {
  const save = useSaveFunnel();
  const [doc, setDoc] = useState<FunnelDocument>(() => draftOf(state));
  const [dirty, setDirty] = useState(false);
  const [newStatus, setNewStatus] = useState("");
  // A version saved elsewhere replaces an untouched draft.
  useEffect(() => {
    if (!dirty) setDoc(draftOf(state));
  }, [state, dirty]);

  const rows = statusRows(state, doc);
  const unmapped = unmappedIn(state, doc);
  const blanks = blankCount(state);
  const nextVersion = (state.current?.version ?? 0) + 1;
  const edit = (next: FunnelDocument) => {
    setDoc(next);
    setDirty(true);
  };
  const stageOptions = [
    { value: NONE, label: "No stage", icon: <StatusMark status="caution" /> },
    ...state.stages.map((s) => ({ value: s, label: STAGE_LABEL[s] })),
  ];

  return (
    <div className="space-y-6">
      <VersionLine state={state} />

      {unmapped.length > 0 && (
        <p className="flex items-baseline gap-2 rounded-lg border px-4 py-3 text-sm">
          <StatusMark status="caution" className="translate-y-[-1px]" />
          <span>
            {unmapped.length === 1
              ? "1 status in the data has no stage yet: "
              : `${unmapped.length} statuses in the data have no stage yet: `}
            <strong className="font-medium">{unmapped.join(", ")}</strong>. Leads with these
            statuses have no stage and are left out of the funnel figures.
          </span>
        </p>
      )}

      {admin && (
        <div className="flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3">
          <span className="text-sm">
            {dirty
              ? `Unsaved changes. Nothing changes until version ${nextVersion} is saved.`
              : state.configured
                ? "Pick a stage for a status to start a new version."
                : "No table is saved yet. Give the statuses their stages and save version 1."}
          </span>
          <span className="ml-auto flex flex-wrap gap-2">
            <Button
              variant="ghost"
              disabled={!dirty || save.isPending}
              onClick={() => {
                setDoc(draftOf(state));
                setDirty(false);
              }}
            >
              Discard changes
            </Button>
            <Button
              disabled={!dirty || save.isPending}
              onClick={() => save.mutate(doc, { onSuccess: () => setDirty(false) })}
            >
              {save.isPending ? "Saving" : `Save as version ${nextVersion}`}
            </Button>
          </span>
        </div>
      )}
      {save.error && <ErrorNote error={save.error} />}
      {save.isSuccess && !dirty && (
        <p className="text-sm text-muted-foreground" role="status">
          Saved version {save.data.version}. Every lead has been re-staged from it.
        </p>
      )}

      <div className="grid gap-6 xl:grid-cols-[22rem_1fr]">
        <Panel
          title="Where the values live"
          description="Which GoHighLevel field carries the status and the project."
        >
          <div className="space-y-4">
            <FieldEditor
              label="Status"
              value={doc.statusField}
              admin={admin}
              onChange={(statusField) => edit({ ...doc, statusField })}
            />
            <FieldEditor
              label="Project"
              value={doc.projectField}
              admin={admin}
              onChange={(projectField) => edit({ ...doc, projectField })}
            />
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground">Note</span>
              {admin ? (
                <Input
                  aria-label="Note"
                  value={doc.note}
                  onChange={(e) => edit({ ...doc, note: e.target.value })}
                  placeholder="Where the table came from"
                />
              ) : (
                <p className="text-sm">
                  {doc.note || <span className="text-muted-foreground">No note</span>}
                </p>
              )}
            </div>
          </div>
        </Panel>

        <Panel
          title="GoHighLevel status to Adx stage"
          description="A stage is Adx's own grouping for funnel maths; GoHighLevel never sees it. Engaged, Meeting, Offer and Won count as qualified."
          bodyClassName="p-0"
        >
          {rows.length === 0 && blanks === 0 ? (
            <EmptyState title="No statuses yet">
              They appear once leads are imported from GoHighLevel. An admin can add a status below
              before the first import.
            </EmptyState>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Status in GoHighLevel</TH>
                  <TH numeric>Leads</TH>
                  <TH>Stage now</TH>
                  <TH className="w-56">{admin ? "Stage" : "Counts as"}</TH>
                </TR>
              </THead>
              <TBody>
                <TR>
                  <TD className="font-medium text-muted-foreground">No status</TD>
                  <TD numeric>{formatNumber(blanks)}</TD>
                  <TD>
                    <StageNow
                      stage={state.current?.document.blankStatusStage ?? null}
                      configured={state.configured}
                    />
                  </TD>
                  <TD>
                    {admin ? (
                      <Select
                        aria-label="Stage for a lead with no status"
                        value={doc.blankStatusStage ?? NONE}
                        onValueChange={(v) =>
                          edit({ ...doc, blankStatusStage: v === NONE ? null : (v as Stage) })
                        }
                        options={stageOptions}
                        className="w-full"
                      />
                    ) : (
                      <span className="text-sm text-muted-foreground">
                        {countsAs(doc.blankStatusStage ?? null)}
                      </span>
                    )}
                  </TD>
                </TR>
                {rows.map((r) => (
                  <TR key={r.key}>
                    <TD className="font-medium">
                      {r.label}
                      {!r.observed && (
                        <span className="block text-xs font-normal text-muted-foreground">
                          Not seen in the data yet
                        </span>
                      )}
                    </TD>
                    <TD numeric>{formatNumber(r.leads)}</TD>
                    <TD>
                      <StageNow stage={r.current} configured={state.configured} />
                    </TD>
                    <TD>
                      {admin ? (
                        <Select
                          aria-label={`Stage for ${r.label}`}
                          value={r.draft ?? NONE}
                          onValueChange={(v) =>
                            edit(withStage(doc, r.label, v === NONE ? null : (v as Stage)))
                          }
                          options={stageOptions}
                          className="w-full"
                        />
                      ) : (
                        <CountsAs row={r} />
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
          {admin && (
            <form
              className="flex flex-wrap items-center gap-2 border-t px-4 py-3"
              onSubmit={(e) => {
                e.preventDefault();
                const raw = newStatus.trim();
                if (!raw) return;
                edit(withStage(doc, raw, "open"));
                setNewStatus("");
              }}
            >
              <Input
                aria-label="A status GoHighLevel will send"
                placeholder="A status GoHighLevel will send, spelled as it spells it"
                value={newStatus}
                onChange={(e) => setNewStatus(e.target.value)}
                className="h-8 w-80"
              />
              <Button type="submit" size="sm" variant="ghost" disabled={!newStatus.trim()}>
                <Plus />
                Add a status
              </Button>
            </form>
          )}
        </Panel>
      </div>
    </div>
  );
}

function VersionLine({ state }: { state: FunnelState }) {
  const c = state.current;
  return (
    <p className="flex flex-wrap items-center gap-2 text-sm">
      {c ? (
        <>
          <StatusMark status="ready" />
          Version {c.version}, saved{" "}
          {formatDate(c.createdAt, { day: "numeric", month: "short", year: "numeric" })}. Saving
          again makes version {c.version + 1} and re-stages every lead; earlier versions are kept.
        </>
      ) : (
        <>
          <StatusMark status="caution" />
          No table is saved yet, so no lead has a stage and funnel figures show Not available.
        </>
      )}
    </p>
  );
}

function StageNow({ stage, configured }: { stage: Stage | null; configured: boolean }) {
  if (stage) return <>{STAGE_LABEL[stage]}</>;
  return <StatusChip status="caution">{configured ? "No stage" : "No table"}</StatusChip>;
}

function CountsAs({ row }: { row: StatusRow }) {
  return <span className="text-sm text-muted-foreground">{countsAs(row.draft)}</span>;
}

function FieldEditor({
  label,
  value,
  admin,
  onChange,
}: {
  label: string;
  value: FieldLocation;
  admin: boolean;
  onChange: (v: FieldLocation) => void;
}) {
  const kind = FIELD_KINDS.find((k) => k.value === value.kind)?.label ?? value.kind;
  return (
    <div className="space-y-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      {admin ? (
        <div className="flex flex-wrap gap-2">
          <Select
            aria-label={`${label} field kind`}
            value={value.kind}
            onValueChange={(v) => onChange({ kind: v, ref: v === "" ? "" : value.ref })}
            options={FIELD_KINDS}
            className="w-56"
          />
          <Input
            aria-label={`${label} field reference`}
            className="w-40 font-mono"
            placeholder={value.kind === "custom_field" ? "field id" : "reference"}
            value={value.ref}
            disabled={value.kind === ""}
            onChange={(e) => onChange({ ...value, ref: e.target.value })}
          />
        </div>
      ) : (
        <p className="text-sm">
          {value.kind === "" ? (
            <span className="text-muted-foreground">Not configured</span>
          ) : (
            <>
              {kind} <span className="font-mono text-xs">{value.ref}</span>
            </>
          )}
        </p>
      )}
    </div>
  );
}
