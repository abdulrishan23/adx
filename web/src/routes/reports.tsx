import { useState } from "react";
import { useReports } from "@/api/adx";
import type { Report } from "@/api/models";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Sheet } from "@/components/ui/sheet";
import { StatusChip } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";

const KIND_LABEL = { weekly: "Weekly", monthly: "Monthly" } as const;

export function ReportsPage() {
  const reports = useReports();
  const [openId, setOpenId] = useState<string | null>(null);
  const open = reports.data?.find((r) => r.id === openId);
  return (
    <>
      <PageHeader title="Reports" />
      {reports.isPending && <LoadingRows rows={4} />}
      {reports.error && <ErrorNote error={reports.error} />}
      {reports.data && reports.data.length === 0 && (
        <EmptyState title="No reports yet">The first weekly report arrives next Monday.</EmptyState>
      )}
      {reports.data && reports.data.length > 0 && (
        <Panel bodyClassName="p-0">
          <ul className="divide-y">
            {reports.data.map((r) => (
              <li key={r.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-5 py-4">
                <div className="min-w-0 flex-1">
                  <div className="font-medium">{r.title}</div>
                  <div className="text-xs text-muted-foreground">
                    {KIND_LABEL[r.kind]} · {r.period} · {formatRelative(r.createdAt)}
                  </div>
                </div>
                <StatusChip status={r.state}>{r.stateLabel}</StatusChip>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={r.state === "pending"}
                  onClick={() => setOpenId(r.id)}
                >
                  Open
                </Button>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      {open && <ReportSheet report={open} onClose={() => setOpenId(null)} />}
    </>
  );
}

function ReportSheet({ report: r, onClose }: { report: Report; onClose: () => void }) {
  return (
    <Sheet
      open
      onOpenChange={(o) => !o && onClose()}
      title={r.title}
      description={`${KIND_LABEL[r.kind]} · ${r.period}`}
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="outline" disabled title="Export lands with the real reports">
            Download PDF
          </Button>
        </div>
      }
    >
      {r.state === "caution" && (
        <p className="mb-4 rounded-md border border-status-caution/50 bg-status-caution/10 px-3 py-2 text-sm">
          Some sources were missing for part of this period.
        </p>
      )}
      <div className="grid grid-cols-2 gap-3">
        {r.figures.map((f) => (
          <div key={f.label} className="rounded-lg border p-3">
            <Eyebrow>{f.label}</Eyebrow>
            <div className="mt-1 font-heading text-xl font-semibold tabular-nums">{f.value}</div>
          </div>
        ))}
      </div>
      <h3 className="mt-6 mb-2 text-sm font-semibold">What happened</h3>
      <ul className="list-disc space-y-2 pl-5 text-sm leading-relaxed">
        {r.summary.map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ul>
    </Sheet>
  );
}
