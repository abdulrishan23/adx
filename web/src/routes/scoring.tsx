import { type FormEvent, useId, useState } from "react";
import { useScoring, useUpdateScoring } from "@/api/adx";
import type { Scoring, Temperature } from "@/api/models";
import { Form } from "@/components/form";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { TemperatureMeter } from "@/components/temperature";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatNumber, formatPercent, formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

const TEMP_LABEL: Record<Temperature, string> = { hot: "Hot", warm: "Warm", cold: "Cold" };

export function ScoringPage() {
  const scoring = useScoring();
  return (
    <>
      <PageHeader title="Scoring" />
      {scoring.isPending && <LoadingRows rows={8} />}
      {scoring.error && <ErrorNote error={scoring.error} />}
      {scoring.data && <Body s={scoring.data} />}
    </>
  );
}

function Body({ s }: { s: Scoring }) {
  const update = useUpdateScoring();
  const hot = s.calibration.find((c) => c.temperature === "hot");
  const warm = s.calibration.find((c) => c.temperature === "warm");
  const lift =
    hot?.qualifiedRate && warm?.qualifiedRate ? hot.qualifiedRate / warm.qualifiedRate : null;
  const maxRate = Math.max(...s.calibration.map((c) => c.qualifiedRate ?? 0), 0.01);

  return (
    <div className="space-y-6">
      <Panel
        title="Calibration"
        description={`Last 90 days · ${formatPercent(s.coverage)} of leads have a status`}
      >
        {lift !== null && (
          <p className="mb-5 text-lg">
            Hot leads qualify <strong className="font-semibold">{lift.toFixed(1)}×</strong> as often
            as warm ones.
          </p>
        )}
        <div className="space-y-3">
          {s.calibration.map((c) => (
            <div
              key={c.temperature}
              className="grid grid-cols-[1fr_5rem_5rem] items-center gap-x-3 gap-y-1.5 text-sm sm:grid-cols-[9rem_1fr_6rem_6rem]"
            >
              <span className="col-span-3 flex items-center gap-2 sm:col-span-1">
                <TemperatureMeter temperature={c.temperature} />
                <span className="text-xs text-muted-foreground">{formatNumber(c.leads)}</span>
              </span>
              <div className="h-2.5 rounded-none bg-muted">
                <div
                  className="h-full rounded-none bg-foreground"
                  style={{ width: `${((c.qualifiedRate ?? 0) / maxRate) * 100}%` }}
                />
              </div>
              <span className="font-mono tabular-nums">
                {formatPercent(c.qualifiedRate)}{" "}
                <span className="text-xs text-muted-foreground">qual.</span>
              </span>
              <span className="font-mono tabular-nums">
                {formatPercent(c.dealRate, 1)}{" "}
                <span className="text-xs text-muted-foreground">deal</span>
              </span>
            </div>
          ))}
        </div>
      </Panel>

      <Thresholds s={s} />

      {update.error && <ErrorNote error={update.error} />}

      <div className="grid gap-4 xl:grid-cols-2">
        <Panel title="Fit" bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Question</TH>
                <TH>Answer</TH>
                <TH numeric className="w-24">
                  Points
                </TH>
              </TR>
            </THead>
            <TBody>
              {s.fit.map((f) => (
                <TR key={f.key}>
                  <TD className="text-muted-foreground">{f.question}</TD>
                  <TD>{f.answer}</TD>
                  <TD numeric>
                    <NumberCell
                      label={`Points for ${f.answer}`}
                      value={f.points}
                      onCommit={(points) => update.mutate({ fit: { key: f.key, points } })}
                    />
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>

        <Panel title="Signals" bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Signal</TH>
                <TH numeric className="w-24">
                  Points
                </TH>
                <TH numeric className="w-20">
                  Cap
                </TH>
                <TH numeric className="w-28">
                  Half-life, days
                </TH>
              </TR>
            </THead>
            <TBody>
              {s.signals.map((g) => (
                <TR key={g.code}>
                  <TD>
                    <span className="font-medium">{g.label}</span>
                    <span className="ml-2 font-mono text-xs text-muted-foreground">{g.code}</span>
                  </TD>
                  <TD numeric>
                    <NumberCell
                      label={`Points for ${g.label}`}
                      value={g.points}
                      onCommit={(points) => update.mutate({ signal: { code: g.code, points } })}
                    />
                  </TD>
                  <TD numeric className="text-muted-foreground">
                    {g.cap}
                  </TD>
                  <TD numeric>
                    <NumberCell
                      label={`Half-life for ${g.label}`}
                      value={g.halfLifeDays}
                      onCommit={(halfLifeDays) =>
                        update.mutate({ signal: { code: g.code, halfLifeDays } })
                      }
                    />
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      </div>

      <p className="text-xs text-muted-foreground">
        Last changed by {s.lastChangedBy} {formatRelative(s.lastChangedAt)}
      </p>
    </div>
  );
}

function Thresholds({ s }: { s: Scoring }) {
  const update = useUpdateScoring();
  const [hot, setHot] = useState(String(s.thresholds.hot));
  const [warm, setWarm] = useState(String(s.thresholds.warm));
  const changed = hot !== String(s.thresholds.hot) || warm !== String(s.thresholds.warm);
  function onSubmit(e: FormEvent) {
    e.preventDefault();
    update.mutate({ thresholds: { hot: Number(hot), warm: Number(warm) } });
  }
  const h = Math.min(100, Number(hot) || 0);
  const w = Math.min(h, Number(warm) || 0);
  return (
    <Panel title="Thresholds">
      <Form onSubmit={onSubmit} className="space-y-4">
        <div className="relative flex h-3 overflow-hidden rounded-none" aria-hidden="true">
          <div className="bg-muted" style={{ width: `${w}%` }} />
          <div className="bg-foreground/40" style={{ width: `${h - w}%` }} />
          <div className="bg-foreground" style={{ width: `${100 - h}%` }} />
        </div>
        <div className="flex flex-wrap items-end gap-4 text-sm">
          <span className="pb-2 text-muted-foreground">Cold below warm</span>
          <ThresholdInput label={TEMP_LABEL.warm} value={warm} onChange={setWarm} />
          <ThresholdInput label={TEMP_LABEL.hot} value={hot} onChange={setHot} />
          {changed && (
            <Button type="submit" size="sm" className="ml-auto" disabled={update.isPending}>
              Save thresholds
            </Button>
          )}
        </div>
        {update.error && <ErrorNote error={update.error} />}
      </Form>
    </Panel>
  );
}

function ThresholdInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const id = useId();
  return (
    <span className="flex items-center gap-2">
      <label htmlFor={id}>{label} from</label>
      <Input
        id={id}
        inputMode="numeric"
        value={value}
        onChange={(e) => onChange(e.target.value.replace(/\D/g, ""))}
        className="h-8 w-16 font-mono"
      />
    </span>
  );
}

// A number that saves when you leave the field or press Enter.
function NumberCell({
  label,
  value,
  onCommit,
}: {
  label: string;
  value: number;
  onCommit: (n: number) => void;
}) {
  const [draft, setDraft] = useState(String(value));
  const commit = () => {
    if (draft === "" || Number(draft) === value) {
      setDraft(String(value));
      return;
    }
    onCommit(Number(draft));
  };
  return (
    <input
      aria-label={label}
      inputMode="numeric"
      value={draft}
      onChange={(e) => setDraft(e.target.value.replace(/\D/g, ""))}
      onBlur={commit}
      onKeyDown={(e) => e.key === "Enter" && (e.currentTarget as HTMLInputElement).blur()}
      className={cn(
        "h-7 w-14 rounded-md border border-transparent bg-transparent px-1.5 text-right font-mono tabular-nums outline-none hover:border-input focus:border-ring focus:bg-card",
      )}
    />
  );
}
