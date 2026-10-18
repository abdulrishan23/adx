import { useState } from "react";
import type { AlertDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  useAlertAction,
  useAlertConfig,
  useAlerts,
  useEvaluateAlerts,
  usePutAlertConfig,
} from "@/engine/api/reports";
import { useSession } from "@/engine/api/session";

const empty: AlertDocument = {
  costSpike: { thresholdPct: null, minSpendMinor: null, baselineDays: 7, minBaselineDays: 3 },
  zeroLeads: { minSpendMinor: null },
  staleIngestion: { maxAgeHours: null },
  note: "",
};

// Observations about the accounts. Every alert says what was seen and that
// nothing was changed. Checks don't run until an admin sets thresholds.
export function AlertsPage() {
  const [state, setState] = useState("open");
  const alerts = useAlerts(state);
  const config = useAlertConfig();
  const session = useSession();
  const act = useAlertAction();
  const put = usePutAlertConfig();
  const evaluate = useEvaluateAlerts();
  const isAdmin = session.data?.role === "admin";
  const canAct = isAdmin || session.data?.role === "marketer";
  const [doc, setDoc] = useState<AlertDocument | null>(null);
  const current = doc ?? config.data?.current?.document ?? empty;

  return (
    <section className="space-y-8">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">Alerts</h1>
        {canAct && (
          <Button
            variant="outline"
            size="sm"
            disabled={evaluate.isPending}
            onClick={() => evaluate.mutate(yesterday())}
          >
            Run checks for yesterday
          </Button>
        )}
      </div>
      {evaluate.data && (
        <p className="text-sm text-muted-foreground">
          {evaluate.data.evaluated
            ? `Checked ${evaluate.data.day}: ${evaluate.data.fired} new, ${evaluate.data.repeated} repeated.`
            : "No checks ran: alerts are not configured."}
          {evaluate.data.skipped.length > 0 && ` Skipped: ${evaluate.data.skipped.join("; ")}`}
        </p>
      )}

      {config.data && config.data.unconfigured.length > 0 && (
        <p className="rounded-lg border p-3 text-sm">
          Not configured: <strong>{config.data.unconfigured.join(", ")}</strong>. Checks without a
          threshold don’t run.
        </p>
      )}

      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground">Show</span>
        {["open", "acknowledged", "resolved", ""].map((s) => (
          <Button
            key={s || "all"}
            variant={state === s ? "default" : "outline"}
            size="xs"
            onClick={() => setState(s)}
          >
            {s || "all"}
          </Button>
        ))}
      </div>
      {alerts.isPending && <p className="text-muted-foreground">Loading…</p>}
      {alerts.error && <p className="text-destructive">{alerts.error.message}</p>}
      {alerts.data && alerts.data.alerts.length === 0 && (
        <p className="text-muted-foreground">Nothing here.</p>
      )}
      {alerts.data && alerts.data.alerts.length > 0 && (
        <ul className="divide-y border-y">
          {alerts.data.alerts.map((a) => (
            <li key={a.id} className="space-y-1 py-3 text-sm">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span>
                  <span
                    className={
                      a.severity === "urgent" ? "font-medium text-destructive" : "font-medium"
                    }
                  >
                    {a.check}
                  </span>{" "}
                  <span className="text-muted-foreground">
                    {a.subjectLabel} · {a.occurrences}× · last{" "}
                    {new Date(a.lastSeenAt).toLocaleString()} · config v{a.configVersion}
                  </span>
                </span>
                {canAct && a.state !== "resolved" && (
                  <span className="flex gap-1">
                    {a.state === "open" && (
                      <Button
                        variant="outline"
                        size="xs"
                        onClick={() => act.mutate({ id: a.id, action: "acknowledge" })}
                      >
                        Acknowledge
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      size="xs"
                      onClick={() => act.mutate({ id: a.id, action: "resolve" })}
                    >
                      Resolve
                    </Button>
                  </span>
                )}
              </div>
              <div>{a.message}</div>
            </li>
          ))}
        </ul>
      )}

      <fieldset className="space-y-3 text-sm" disabled={!isAdmin}>
        <legend className="font-medium">Thresholds (owner’s input)</legend>
        <Field
          label="Cost spike: percent above average"
          value={current.costSpike.thresholdPct}
          onChange={(v) =>
            setDoc({ ...current, costSpike: { ...current.costSpike, thresholdPct: v } })
          }
        />
        <Field
          label="Cost spike: minimum spend (minor units)"
          value={current.costSpike.minSpendMinor}
          onChange={(v) =>
            setDoc({ ...current, costSpike: { ...current.costSpike, minSpendMinor: v } })
          }
        />
        <Field
          label="Zero leads: minimum spend (minor units)"
          value={current.zeroLeads.minSpendMinor}
          onChange={(v) => setDoc({ ...current, zeroLeads: { minSpendMinor: v } })}
        />
        <Field
          label="Stale ingestion: maximum age (hours)"
          value={current.staleIngestion.maxAgeHours}
          onChange={(v) => setDoc({ ...current, staleIngestion: { maxAgeHours: v } })}
        />
        {isAdmin && (
          <div className="flex items-center gap-2">
            <Button size="sm" disabled={put.isPending} onClick={() => put.mutate(current)}>
              Save as new version
            </Button>
            {put.error && <span className="text-destructive">{put.error.message}</span>}
            {put.data && (
              <span className="text-muted-foreground">Saved version {put.data.version}.</span>
            )}
          </div>
        )}
      </fieldset>
    </section>
  );
}

function Field({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number | null;
  onChange: (v: number | null) => void;
}) {
  const id = `alert-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <label htmlFor={id} className="w-72 text-muted-foreground">
        {label}
      </label>
      <Input
        id={id}
        type="number"
        className="max-w-40"
        placeholder="Unconfigured"
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))}
      />
    </div>
  );
}

function yesterday(): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - 1);
  return d.toISOString().slice(0, 10);
}
