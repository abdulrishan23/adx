import { Popover } from "@base-ui/react/popover";
import { CalendarDays, ChevronDown } from "lucide-react";
import { useState } from "react";
import {
  type AnalyticsSearch,
  analyticsPeriodLabel,
  comparisonRange,
  dateShift,
  dubaiToday,
  rangeError,
} from "@/lib/analytics";
import { formatDate } from "@/lib/format";
import { Button } from "./ui/button";
import { Select } from "./ui/select";

const dateLabel = (date: string) => formatDate(`${date}T00:00:00Z`);
const datesLabel = (from: string, to: string) =>
  `${dateLabel(from)} – ${formatDate(`${to}T00:00:00Z`, { day: "numeric", month: "short", year: "numeric" })}`;
type Period = "month" | "lastMonth" | "7" | "30" | "90" | "custom";

function periodFor(search: AnalyticsSearch): Period {
  const today = dubaiToday();
  if (analyticsPeriodLabel(search) === "Month to date") return "month";
  const last = dateShift(`${today.slice(0, 7)}-01`, -1);
  if (search.from === `${last.slice(0, 7)}-01` && search.to === last) return "lastMonth";
  for (const days of [7, 30, 90] as const)
    if (search.to === dateShift(today, -1) && search.from === dateShift(search.to, -(days - 1)))
      return String(days) as Period;
  return "custom";
}

const PERIODS = [
  { value: "month", label: "Month to date" },
  { value: "lastMonth", label: "Last month" },
  { value: "7", label: "Last 7 days" },
  { value: "30", label: "Last 30 days" },
  { value: "90", label: "Last 90 days" },
  { value: "custom", label: "Custom dates" },
] as const;

export function AnalyticsDatePicker({
  search,
  onChange,
}: {
  search: AnalyticsSearch;
  onChange: (next: Partial<AnalyticsSearch>) => void;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(search);
  const [period, setPeriod] = useState<Period>(() => periodFor(search));
  const [submitted, setSubmitted] = useState(false);
  const yesterday = dateShift(dubaiToday(), -1);
  const draftError = rangeError(draft);
  const selectedComparison = search.compare === "none" ? null : comparisonRange(search);
  const preview = !draftError && draft.compare !== "none" ? comparisonRange(draft) : null;
  const periodLabel = PERIODS.find((p) => p.value === periodFor(search))?.label;
  const label =
    periodFor(search) === "custom"
      ? datesLabel(search.from, search.to)
      : `${periodLabel} · ${dateLabel(search.from)} – ${dateLabel(search.to)}`;
  function choosePeriod(value: Period) {
    setPeriod(value);
    setSubmitted(false);
    if (value === "custom") return;
    const today = dubaiToday();
    const start = `${today.slice(0, 7)}-01`;
    const to = value === "lastMonth" ? dateShift(start, -1) : yesterday;
    const from =
      value === "month"
        ? start
        : value === "lastMonth"
          ? `${to.slice(0, 7)}-01`
          : dateShift(to, -(Number(value) - 1));
    setDraft({
      ...draft,
      from,
      to,
      compare: value === "month" || value === "lastMonth" ? "month" : "previous",
      comparisonFrom: undefined,
      comparisonTo: undefined,
    });
  }
  function chooseComparison(compare: AnalyticsSearch["compare"]) {
    const suggested = comparisonRange(rangeError({ ...draft, compare: "none" }) ? search : draft);
    setDraft({
      ...draft,
      compare,
      comparisonFrom: compare === "custom" ? (draft.comparisonFrom ?? suggested.from) : undefined,
      comparisonTo: compare === "custom" ? (draft.comparisonTo ?? suggested.to) : undefined,
    });
    setSubmitted(false);
  }
  return (
    <Popover.Root
      open={open}
      onOpenChange={(next) => {
        if (next) {
          setDraft(search);
          setPeriod(periodFor(search));
          setSubmitted(false);
        }
        setOpen(next);
      }}
    >
      <Popover.Trigger
        aria-label="Reporting dates"
        className="flex h-10 max-w-full items-center gap-2 border bg-card px-3 py-1 text-left outline-none hover:border-ring focus-visible:ring-2 focus-visible:ring-ring"
      >
        <CalendarDays className="size-4 shrink-0 text-muted-foreground" />
        <span className="min-w-0">
          <span className="block text-sm leading-4 font-medium">{label}</span>
          {selectedComparison && (
            <span className="block text-[10px] leading-3 text-muted-foreground">
              Compared with {datesLabel(selectedComparison.from, selectedComparison.to)}
            </span>
          )}
        </span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner align="end" sideOffset={8} collisionPadding={12} className="z-50">
          <Popover.Popup className="max-h-(--available-height) w-[420px] max-w-[calc(100vw-24px)] overflow-y-auto border bg-popover text-popover-foreground shadow-xl outline-none">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                setSubmitted(true);
                if (draftError) return;
                onChange({
                  from: draft.from,
                  to: draft.to,
                  compare: draft.compare,
                  comparisonFrom: draft.compare === "custom" ? draft.comparisonFrom : undefined,
                  comparisonTo: draft.compare === "custom" ? draft.comparisonTo : undefined,
                });
                setOpen(false);
              }}
            >
              <div className="border-b px-5 py-4">
                <Popover.Title className="font-heading text-lg font-semibold">
                  Reporting dates
                </Popover.Title>
              </div>
              <div className="space-y-5 p-5">
                <div className="space-y-2">
                  <p className="text-sm font-medium">Period</p>
                  <Select
                    aria-label="Reporting period"
                    value={period}
                    onValueChange={choosePeriod}
                    options={PERIODS.map((p) => ({
                      ...p,
                      disabled: p.value === "month" && dubaiToday().endsWith("-01"),
                    }))}
                    className="w-full"
                  />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <DateField
                    label="Start date"
                    value={draft.from}
                    max={yesterday}
                    onChange={(from) => {
                      setDraft({ ...draft, from });
                      setPeriod("custom");
                    }}
                  />
                  <DateField
                    label="End date"
                    value={draft.to}
                    max={yesterday}
                    onChange={(to) => {
                      setDraft({ ...draft, to });
                      setPeriod("custom");
                    }}
                  />
                </div>
                <div className="space-y-2 border-t pt-4">
                  <p className="text-sm font-medium">Compare with</p>
                  <Select
                    aria-label="Comparison period"
                    value={draft.compare}
                    onValueChange={chooseComparison}
                    options={[
                      {
                        value: "month",
                        label: period === "lastMonth" ? "Previous month" : "Same days last month",
                      },
                      { value: "previous", label: "Previous period" },
                      { value: "custom", label: "Custom dates" },
                      { value: "none", label: "Off" },
                    ]}
                    className="w-full"
                  />
                </div>
                {draft.compare === "custom" && (
                  <div className="grid grid-cols-2 gap-3">
                    <DateField
                      label="Comparison start"
                      value={draft.comparisonFrom ?? ""}
                      max={yesterday}
                      onChange={(comparisonFrom) => setDraft({ ...draft, comparisonFrom })}
                    />
                    <DateField
                      label="Comparison end"
                      value={draft.comparisonTo ?? ""}
                      max={yesterday}
                      onChange={(comparisonTo) => setDraft({ ...draft, comparisonTo })}
                    />
                  </div>
                )}
                {!draftError && (
                  <div className="space-y-1 border bg-muted/30 px-3 py-3 text-sm">
                    <p className="font-medium">{datesLabel(draft.from, draft.to)}</p>
                    {preview ? (
                      <p className="text-xs text-muted-foreground">
                        Compared with {datesLabel(preview.from, preview.to)}
                      </p>
                    ) : (
                      <p className="text-xs text-muted-foreground">No comparison</p>
                    )}
                  </div>
                )}
                {submitted && draftError && (
                  <p role="alert" className="text-sm text-destructive">
                    {draftError}
                  </p>
                )}
              </div>
              <div className="flex justify-end gap-2 border-t px-5 py-3">
                <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit">Apply</Button>
              </div>
            </form>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}

function DateField({
  label,
  value,
  max,
  onChange,
}: {
  label: string;
  value: string;
  max: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="block min-w-0 text-xs text-muted-foreground">
      {label}
      <input
        type="date"
        aria-label={label}
        required
        max={max}
        value={value}
        onInput={(e) => onChange(e.currentTarget.value)}
        onChange={(e) => onChange(e.currentTarget.value)}
        className="mt-1.5 block h-9 w-full min-w-0 border bg-card px-2 text-sm text-foreground outline-none focus-visible:border-foreground focus-visible:ring-2 focus-visible:ring-ring"
      />
    </label>
  );
}
