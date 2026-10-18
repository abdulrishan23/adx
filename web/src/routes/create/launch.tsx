import { getRouteApi, Link } from "@tanstack/react-router";
import { useState } from "react";
import { type Launch, useFixBlocker, useLaunches } from "@/api/create";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";
import { BrandPicker, SampleChip } from "./shared";

const route = getRouteApi("/app/create/launch");

// Launch is the plan becoming live campaigns: readiness checks, then
// generate, validate, submit, platform review, delivering. Each is its own
// milestone, so "launched" never hides "rejected by Meta". Create owns this
// first launch; what happens to a campaign afterwards is Perform's (BD-58),
// and Perform › Campaigns arrives here with ?project= for its brand.
export function LaunchPage() {
  const launches = useLaunches();
  const fromCampaigns = route.useSearch().project;
  const [brandId, setBrandId] = useState<number>(fromCampaigns ?? 1);
  const l = launches.data?.find((x) => x.projectId === brandId);
  const have = launches.data?.map((x) => x.projectId) ?? [];
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            Launch <SampleChip phase={3} />
          </span>
        }
        subtitle="From approved plan to delivering campaigns. Adx generates and submits; a person approves spend and fixes what a platform rejects."
        actions={<BrandPicker value={brandId} onChange={setBrandId} only={have} />}
      />
      {launches.isPending && <LoadingRows rows={8} />}
      {launches.error && <ErrorNote error={launches.error} />}
      {launches.data && !l && (
        <Panel>
          <p className="text-sm text-muted-foreground">
            Nothing to launch for this brand yet. It starts when a media plan is approved.
          </p>
        </Panel>
      )}
      {l && <LaunchBody launch={l} />}
    </>
  );
}

function LaunchBody({ launch: l }: { launch: Launch }) {
  const fix = useFixBlocker();
  const open = l.blockers.filter((b) => !b.fixed);
  const ready = l.readiness.filter((r) => r.ok).length;
  return (
    <div className="space-y-5">
      {open.length > 0 && (
        <Panel
          title={`${open.length} ${open.length === 1 ? "thing" : "things"} blocking ${l.projectName}`}
          description="Adx can't change ad sets until phase 3's writes are on, so it says exactly what to do."
          className="border-status-error/50"
          bodyClassName="p-0"
        >
          <ul className="divide-y">
            {l.blockers.map((b) => (
              <li key={b.id} className="flex items-start gap-4 px-5 py-3">
                <StatusMark status={b.fixed ? "ready" : "error"} className="mt-2" />
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2 font-medium">
                    <PlatformTag platform={b.platform} /> {b.target}
                  </p>
                  <p className="text-sm text-muted-foreground">
                    {b.reason}. {b.fixed ? "Fixed." : b.fix}
                  </p>
                </div>
                {!b.fixed && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={fix.isPending}
                    onClick={() => fix.mutate({ projectId: l.projectId, blockerId: b.id })}
                  >
                    Mark fixed and resubmit
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </Panel>
      )}

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel
          title="Readiness"
          description={`${ready} of ${l.readiness.length} checks pass. All must, before Adx submits.`}
          bodyClassName="p-0"
        >
          <ul className="divide-y">
            {l.readiness.map((r) => (
              <li key={r.label} className="flex items-start gap-3 px-5 py-3">
                <StatusMark status={r.ok ? "ready" : "caution"} className="mt-2" />
                <div>
                  <p className="text-sm font-medium">{r.label}</p>
                  <p className="text-xs text-muted-foreground">{r.note}</p>
                </div>
              </li>
            ))}
          </ul>
        </Panel>
        <Panel
          title="Where it is"
          description={
            l.planId ? (
              <>
                From{" "}
                <Link
                  to="/create/plans/$planId"
                  params={{ planId: l.planId }}
                  className="underline underline-offset-4"
                >
                  the approved plan
                </Link>
                . Each step is its own milestone.
              </>
            ) : (
              "Waits for an approved plan."
            )
          }
          bodyClassName="p-0"
        >
          <ol className="divide-y">
            {l.steps.map((s, i) => (
              <li key={s.label} className="flex items-start gap-3 px-5 py-3">
                <span className="w-5 shrink-0 font-mono text-xs text-muted-foreground">
                  {i + 1}
                </span>
                <StatusMark status={s.status} className="mt-2" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">{s.label}</p>
                  {s.note && <p className="text-xs text-muted-foreground">{s.note}</p>}
                </div>
                {s.at && (
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    {formatRelative(s.at)}
                  </span>
                )}
              </li>
            ))}
          </ol>
        </Panel>
      </div>
      <p className="text-sm text-muted-foreground">
        Target from the CEO's flow: plan approved to live in under an hour.{" "}
        <StatusChip status="pending" className="font-sans normal-case tracking-normal">
          Measured from phase 3
        </StatusChip>
      </p>
    </div>
  );
}
