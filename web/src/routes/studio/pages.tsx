import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft, ExternalLink } from "lucide-react";
import { useState } from "react";
import {
  usePage,
  usePagePreview,
  usePageReadiness,
  usePageSubmissions,
  usePages,
  usePublishPage,
  useRollbackPage,
  useTestSubmission,
} from "@/api/pages";
import type {
  ClaimCheck,
  Deployment,
  PageDetail,
  PageReadinessIssue,
  PageSummary,
  Submission,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button, buttonVariants } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDateTime, formatRelative } from "@/lib/format";

const route = getRouteApi("/app/studio/pages/$pageId");

// A page is live when a deployment is published; a draft until then. The
// list can't tell a broken form from here, so the page view says that.
function standing(p: PageSummary): { status: Status; label: string } {
  if (p.publishedVersion === null) return { status: "pending", label: "Draft" };
  if (p.publishedVersion < p.latestVersion) {
    return { status: "caution", label: `Live v${p.publishedVersion}, draft v${p.latestVersion}` };
  }
  return { status: "ready", label: `Live v${p.publishedVersion}` };
}

// Landing pages are built from approved facts, with their forms wired to
// GoHighLevel, so what the ad promises and what the page says agree.
export function PagesPage() {
  const pages = usePages();
  const list = pages.data?.pages ?? [];
  return (
    <>
      <PageHeader
        title="Landing pages"
        subtitle="One per brand, language and offer, served by Adx. A page is ready when it's live, its form reaches someone, and every claim on it is an approved fact."
        actions={
          <Link to="/engine/pages" className={buttonVariants({ variant: "outline", size: "sm" })}>
            New page
          </Link>
        }
      />
      {pages.isPending && <LoadingRows rows={5} />}
      {pages.error && <ErrorNote error={pages.error} />}
      {pages.data && list.length === 0 && (
        <Panel>
          <EmptyState title="No pages yet">
            Start one on the{" "}
            <Link to="/engine/pages" className="underline underline-offset-4">
              engine's pages screen
            </Link>{" "}
            from a brand and an offer, then publish and test it here.
          </EmptyState>
        </Panel>
      )}
      {list.length > 0 && (
        <Panel bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Page</TH>
                <TH>Brand</TH>
                <TH>Standing</TH>
                <TH>Language</TH>
                <TH>Updated</TH>
              </TR>
            </THead>
            <TBody>
              {list.map((p) => {
                const s = standing(p);
                return (
                  <TR key={p.id}>
                    <TD>
                      <Link
                        to="/studio/pages/$pageId"
                        params={{ pageId: String(p.id) }}
                        className="font-medium hover:underline"
                        dir={p.language === "ar" ? "rtl" : undefined}
                        lang={p.language === "ar" ? "ar" : undefined}
                      >
                        {p.name}
                      </Link>
                      <p className="font-mono text-xs text-muted-foreground">{p.publicUrl}</p>
                    </TD>
                    <TD>
                      {p.brandName}
                      {p.projectName && (
                        <span className="text-muted-foreground"> · {p.projectName}</span>
                      )}
                    </TD>
                    <TD>
                      <StatusChip status={s.status}>{s.label}</StatusChip>
                    </TD>
                    <TD>{p.language === "ar" ? "Arabic" : "English"}</TD>
                    <TD className="text-muted-foreground">{formatRelative(p.updatedAt)}</TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </Panel>
      )}
      {pages.data && !pages.data.publishing.external.available && (
        <p className="mt-4 text-xs text-muted-foreground">
          Pages are served by Adx ({pages.data.publishing.local}). Publishing to an outside host
          waits on {pages.data.publishing.external.unblocks}.
        </p>
      )}
    </>
  );
}

export function PagePage() {
  const { pageId } = route.useParams();
  const id = Number(pageId);
  const page = usePage(id);
  const [picked, setPicked] = useState<number | null>(null);
  if (page.isPending) return <LoadingRows rows={8} />;
  if (page.error) return <ErrorNote error={page.error} />;
  if (!page.data) return <LoadingRows rows={8} />;
  const d = page.data;
  const versions = [...d.versions].sort((a, b) => b.version - a.version);
  const latest = versions[0];
  const shown = versions.find((x) => x.version === picked) ?? latest;
  return (
    <>
      <Link
        to="/studio/pages"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Landing pages
      </Link>
      <PageHeader
        title={<span dir={d.page.language === "ar" ? "rtl" : undefined}>{d.page.name}</span>}
        subtitle={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="font-mono text-xs">{d.publicUrl}</span>
            {d.current ? (
              <StatusChip status="ready">Live v{d.current.version}</StatusChip>
            ) : (
              <StatusChip status="pending">Not published</StatusChip>
            )}
          </span>
        }
        actions={
          <div className="flex items-center gap-2">
            {versions.length > 1 && (
              <Select
                aria-label="Version"
                value={String(shown?.version ?? "")}
                onValueChange={(x) => setPicked(Number(x))}
                options={versions.map((x) => ({
                  value: String(x.version),
                  label: `v${x.version}${d.current?.version === x.version ? " · live" : ""}`,
                }))}
              />
            )}
            {d.current && (
              <a
                href={d.publicUrl}
                target="_blank"
                rel="noreferrer"
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                <ExternalLink /> Open
              </a>
            )}
          </div>
        }
      />
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="space-y-5">
          {shown ? (
            <Preview pageId={id} version={shown.version} language={d.page.language} />
          ) : (
            <Panel>
              <EmptyState title="No version yet" />
            </Panel>
          )}
          <Submissions pageId={id} />
        </div>
        <div className="space-y-5">
          <Readiness detail={d} />
          <Deployments detail={d} latestVersion={latest?.version ?? null} />
        </div>
      </div>
    </>
  );
}

function Preview({
  pageId,
  version,
  language,
}: {
  pageId: number;
  version: number;
  language: string;
}) {
  const preview = usePagePreview(pageId, version);
  return (
    <Panel
      title={`Version ${version}`}
      description="Rendered as a visitor sees it."
      bodyClassName="p-0"
    >
      {preview.error && (
        <div className="p-5">
          <ErrorNote error={preview.error} />
        </div>
      )}
      <iframe
        title={`Version ${version}`}
        srcDoc={preview.data ?? ""}
        sandbox=""
        dir={language === "ar" ? "rtl" : undefined}
        className="h-[640px] w-full bg-white"
      />
    </Panel>
  );
}

// Ready means live, a form that reaches someone and was tested on this
// version, and approved claims. It is not a launch.
function Readiness({ detail }: { detail: PageDetail }) {
  const session = useSession();
  const readiness = usePageReadiness(detail.page.id);
  const test = useTestSubmission();
  const r = readiness.data;
  const canEdit = session.role !== "viewer";
  const live = detail.current;
  const form = detail.versions.find((v) => v.version === live?.version)?.document.form;
  const sample: Record<string, string> = {};
  for (const f of form?.fields ?? []) {
    if (f.kind === "email") sample[f.name] = "test@reportage.example";
    else if (f.kind === "phone") sample[f.name] = "+971500000000";
    else if (f.kind === "select") sample[f.name] = f.options[0] ?? "";
    else sample[f.name] = "Adx test";
  }
  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          {r && <StatusMark status={r.ready ? "ready" : r.blockers.length ? "error" : "caution"} />}
          {r ? (r.ready ? "Ready for launch" : "Not ready for launch") : "Readiness"}
        </span>
      }
      description="Live, a form that reaches someone and was tested on this version, approved claims."
    >
      {readiness.isPending && <LoadingRows rows={3} />}
      {readiness.error && <ErrorNote error={readiness.error} />}
      {r && (
        <div className="space-y-4">
          {r.blockers.length > 0 && (
            <ul className="space-y-1.5">
              {r.blockers.map((b) => (
                <Issue key={b.code + b.message} issue={b} blocking />
              ))}
            </ul>
          )}
          {r.warnings.length > 0 && (
            <ul className="space-y-1.5">
              {r.warnings.map((w) => (
                <Issue key={w.code + w.message} issue={w} />
              ))}
            </ul>
          )}
          {r.claims.length > 0 && (
            <div>
              <Eyebrow className="mb-2">Claims on the page</Eyebrow>
              <ul className="space-y-1.5">
                {r.claims.map((c) => (
                  <Claim key={c.text + c.kind} claim={c} />
                ))}
              </ul>
            </div>
          )}
          <div className="space-y-2 border-t pt-4 text-sm">
            {r.lastTest ? (
              <p className="text-muted-foreground">
                Last test on the live version: {r.lastTest.state}
                {r.lastTest.reason ? ` (${r.lastTest.reason})` : ""},{" "}
                {formatRelative(r.lastTest.receivedAt)}.
              </p>
            ) : (
              <p className="text-muted-foreground">No test sent on the live version yet.</p>
            )}
            {canEdit && live && detail.testToken && (
              <Button
                size="sm"
                variant="outline"
                disabled={test.isPending}
                onClick={() =>
                  test.mutate({
                    slug: detail.page.slug,
                    token: detail.testToken,
                    project: detail.page.projectId ?? 0,
                    fields: sample,
                  })
                }
              >
                Send a test submission
              </Button>
            )}
            {test.error && <p className="text-destructive">{test.error.message}</p>}
            {test.data && <p className="text-muted-foreground">Recorded and marked as a test.</p>}
          </div>
        </div>
      )}
    </Panel>
  );
}

function Issue({ issue, blocking }: { issue: PageReadinessIssue; blocking?: boolean }) {
  return (
    <li className="flex items-start gap-2 text-sm">
      <StatusMark status={blocking ? "error" : "caution"} className="mt-1.5" />
      <span>{issue.message}</span>
    </li>
  );
}

function Claim({ claim }: { claim: ClaimCheck }) {
  const ok = claim.status === "approved";
  return (
    <li className="flex items-start gap-2 text-sm">
      <StatusMark status={ok ? "ready" : "caution"} className="mt-1.5" />
      <span>
        {claim.text}
        <span className="text-muted-foreground">
          {" "}
          · {claim.kind}
          {claim.detail ? ` · ${claim.detail}` : ""}
        </span>
      </span>
    </li>
  );
}

// Publish makes a version live at the public URL; roll back republishes the
// one before it, the exact bytes, as a new deployment.
function Deployments({
  detail,
  latestVersion,
}: {
  detail: PageDetail;
  latestVersion: number | null;
}) {
  const session = useSession();
  const publish = usePublishPage(detail.page.id);
  const rollback = useRollbackPage(detail.page.id);
  const canEdit = session.role !== "viewer";
  const liveIsLatest = detail.current?.version === latestVersion;
  return (
    <Panel
      title="Deployments"
      action={
        canEdit &&
        latestVersion !== null && (
          <div className="flex gap-2">
            {detail.current && detail.deployments.length > 1 && (
              <Button
                size="sm"
                variant="outline"
                disabled={rollback.isPending}
                onClick={() => rollback.mutate()}
              >
                Roll back
              </Button>
            )}
            <Button
              size="sm"
              disabled={publish.isPending || liveIsLatest}
              onClick={() => publish.mutate(latestVersion)}
            >
              Publish v{latestVersion}
            </Button>
          </div>
        )
      }
      bodyClassName="p-0"
    >
      {(publish.error ?? rollback.error) && (
        <div className="p-5">
          <ErrorNote error={publish.error ?? rollback.error ?? new Error("Failed")} />
        </div>
      )}
      {detail.deployments.length === 0 ? (
        <EmptyState title="Never published" />
      ) : (
        <ul className="divide-y">
          {detail.deployments.map((dp) => (
            <DeploymentRow key={dp.id} dp={dp} />
          ))}
        </ul>
      )}
    </Panel>
  );
}

function DeploymentRow({ dp }: { dp: Deployment }) {
  const status: Status =
    dp.state === "published" ? "ready" : dp.state === "failed" ? "error" : "pending";
  const label = dp.state === "published" ? "Live" : dp.state === "failed" ? "Failed" : "Superseded";
  return (
    <li className="flex items-start justify-between gap-3 px-5 py-3 text-sm">
      <div>
        <p className="flex items-center gap-2">
          <StatusMark status={status} />
          <span className="font-medium">v{dp.version}</span>
          <span className="text-muted-foreground">{label}</span>
        </p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {formatDateTime(dp.deployedAt)}
          {dp.rolledBackFromVersion !== null && ` · rolled back from v${dp.rolledBackFromVersion}`}
          {dp.error && ` · ${dp.error}`}
        </p>
      </div>
      <span className="font-mono text-[0.65rem] text-muted-foreground">
        {dp.outputSha256.slice(0, 10)}
      </span>
    </li>
  );
}

function Submissions({ pageId }: { pageId: number }) {
  const subs = usePageSubmissions(pageId);
  const list = subs.data?.submissions ?? [];
  return (
    <Panel
      title="Form submissions"
      description="Test sends are marked and never count as enquiries."
      bodyClassName="p-0"
    >
      {subs.isPending && (
        <div className="p-5">
          <LoadingRows rows={3} />
        </div>
      )}
      {subs.error && (
        <div className="p-5">
          <ErrorNote error={subs.error} />
        </div>
      )}
      {subs.data && list.length === 0 && <EmptyState title="Nothing submitted yet" />}
      {list.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Received</TH>
              <TH>Outcome</TH>
              <TH>Fields</TH>
              <TH>Came from</TH>
            </TR>
          </THead>
          <TBody>
            {list.map((s) => (
              <SubmissionRow key={s.id} s={s} />
            ))}
          </TBody>
        </Table>
      )}
    </Panel>
  );
}

function SubmissionRow({ s }: { s: Submission }) {
  const rejected = s.state === "rejected";
  return (
    <TR className={rejected ? "text-muted-foreground" : undefined}>
      <TD>{formatDateTime(s.receivedAt)}</TD>
      <TD>
        <span className="flex items-center gap-2">
          <StatusMark status={rejected ? "error" : s.test ? "pending" : "ready"} />
          {s.test ? "Test" : s.state}
          {s.reason && <span className="text-muted-foreground">· {s.reason}</span>}
        </span>
      </TD>
      <TD className="max-w-md truncate">
        {Object.entries(s.fields ?? {})
          .map(([k, v]) => `${k}: ${v}`)
          .join(" · ")}
      </TD>
      <TD className="max-w-[14rem] truncate font-mono text-xs text-muted-foreground">
        {Object.entries(s.attribution ?? {})
          .map(([k, v]) => `${k}=${v}`)
          .join(" ") || "—"}
      </TD>
    </TR>
  );
}
