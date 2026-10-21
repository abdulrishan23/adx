import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft, ExternalLink, Sparkles } from "lucide-react";
import { useState } from "react";
import { useBrand, useBrands } from "@/api/brands";
import {
  useDraftPage,
  usePage,
  usePagePreview,
  usePageReadiness,
  usePageSubmissions,
  usePublishPage,
  useRollbackPage,
  useTestSubmission,
} from "@/api/pages";
import type {
  ClaimCheck,
  Deployment,
  PageDetail,
  PageReadinessIssue,
  Submission,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { Form } from "@/components/form";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDateTime, formatRelative } from "@/lib/format";

const route = getRouteApi("/app/studio/pages/$pageId");

// Adx writes the page from the brand's approved facts, the offer and the
// studio's pictures for the project, in English and Arabic as twins that
// link to each other. Each lands as a draft version to edit and publish.
export function AskForPage({
  fixed,
  campaignId = 0,
  offerId: fixedOffer,
}: {
  fixed?: { brandId: number; projectId: number };
  campaignId?: number;
  offerId?: number;
}) {
  const session = useSession();
  const brands = useBrands();
  const [brandId, setBrandId] = useState<number | null>(fixed?.brandId ?? null);
  const brand = useBrand(brandId);
  const [projectId, setProjectId] = useState<number | null>(fixed?.projectId ?? null);
  const [offerId, setOfferId] = useState<number>(fixedOffer ?? 0);
  const [language, setLanguage] = useState<"" | "en" | "ar">("");
  const [instructions, setInstructions] = useState("");
  const draft = useDraftPage();
  if (session.role === "viewer") return null;
  const own = brands.data?.brands.filter((b) => b.kind === "own") ?? [];
  const projects = brand.data?.projects ?? [];
  const offers = (brand.data?.offers ?? []).filter(
    (o) => o.projectId === null || o.projectId === projectId,
  );
  return (
    <Panel
      title="Ask Adx for a page"
      description="Written from the approved facts, the offer and the project's pictures, in English and Arabic as twins. Figures the facts don't state are left out and said below."
      className="mb-5"
    >
      <Form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (projectId === null) return;
          draft.mutate({
            projectId,
            language,
            slug: "",
            name: "",
            offerId,
            instructions,
            campaignId,
            pageId: 0,
          });
        }}
      >
        {!fixed && (
          <>
            <Select
              aria-label="Brand"
              placeholder="Brand"
              value={brandId === null ? null : String(brandId)}
              onValueChange={(v) => {
                setBrandId(Number(v));
                setProjectId(null);
                setOfferId(0);
              }}
              options={own.map((b) => ({ value: String(b.id), label: b.name }))}
            />
            <Select
              aria-label="Project"
              placeholder="Project"
              value={projectId === null ? null : String(projectId)}
              onValueChange={(v) => setProjectId(Number(v))}
              options={projects.map((p) => ({ value: String(p.id), label: p.name }))}
              disabled={brandId === null}
            />
          </>
        )}
        <Select
          aria-label="Offer"
          value={String(offerId)}
          onValueChange={(v) => setOfferId(Number(v))}
          options={[
            { value: "0", label: "No offer" },
            ...offers.map((o) => ({ value: String(o.id), label: o.name })),
          ]}
          disabled={projectId === null}
        />
        <Select
          aria-label="Language"
          value={language}
          onValueChange={(v) => setLanguage(v as "" | "en" | "ar")}
          options={[
            { value: "", label: "English and Arabic" },
            { value: "en", label: "English only" },
            { value: "ar", label: "Arabic only" },
          ]}
        />
        <Input
          aria-label="Instructions"
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
          placeholder="Angle, audience, anything to avoid"
          className="w-72"
        />
        <Button type="submit" disabled={draft.isPending || projectId === null}>
          <Sparkles />
          {draft.isPending ? "Writing" : "Write"}
        </Button>
        {draft.error && <p className="w-full text-sm text-destructive">{draft.error.message}</p>}
        {draft.data && (
          <div className="w-full text-sm text-muted-foreground">
            <p>
              Drafted{" "}
              {draft.data.pages.map((d, i) => (
                <span key={d.page.id}>
                  {i > 0 && " and "}
                  <Link
                    to="/studio/pages/$pageId"
                    params={{ pageId: String(d.page.id) }}
                    className="underline underline-offset-4"
                  >
                    {d.page.name}
                  </Link>
                </span>
              ))}
              . Review, then publish.
            </p>
            {draft.data.pages
              .flatMap((d) => d.notes)
              .map((n) => (
                <p key={n}>{n}</p>
              ))}
          </div>
        )}
      </Form>
    </Panel>
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
      {d.page.projectId !== null && d.page.campaignId !== null ? (
        <Link
          to="/projects/$projectId/campaigns/$campaignId"
          params={{ projectId: String(d.page.projectId), campaignId: String(d.page.campaignId) }}
          search={{ stage: "page" }}
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Campaign
        </Link>
      ) : d.page.projectId !== null ? (
        <Link
          to="/projects/$projectId"
          params={{ projectId: String(d.page.projectId) }}
          search={{ tab: "pages" }}
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Unfiled pages
        </Link>
      ) : (
        <Link
          to="/projects"
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Projects
        </Link>
      )}
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
