import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Plus } from "lucide-react";
import { useState } from "react";
import { useBrand, useBrands } from "@/api/brands";
import { useCreateCampaign, useProjectCampaigns } from "@/api/campaigns";
import { usePlans } from "@/api/plans";
import type { CampaignInput, CampaignSummary } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { useProjects } from "@/engine/api/projects";
import { formatDate, formatMoneyWhole } from "@/lib/format";
import { BriefForm, STATE } from "./campaign";

function CampaignTable({ rows, showProject }: { rows: CampaignSummary[]; showProject?: boolean }) {
  return (
    <Panel bodyClassName="p-0">
      <Table>
        <THead>
          <TR>
            <TH>Campaign</TH>
            {showProject && <TH>Project</TH>}
            <TH>Window</TH>
            <TH className="text-right">Budget</TH>
            <TH>Stages</TH>
            <TH>State</TH>
          </TR>
        </THead>
        <TBody>
          {rows.map((c) => {
            const state = STATE[c.state] ?? { label: c.state, status: "pending" as Status };
            return (
              <TR key={c.id}>
                <TD>
                  <Link
                    to="/projects/$projectId/campaigns/$campaignId"
                    params={{ projectId: String(c.projectId), campaignId: String(c.id) }}
                    className="font-medium hover:underline"
                  >
                    {c.name}
                  </Link>
                </TD>
                {showProject && <TD className="text-muted-foreground">{c.projectName}</TD>}
                <TD className="text-muted-foreground">
                  {[c.startsOn && formatDate(c.startsOn), c.endsOn && formatDate(c.endsOn)]
                    .filter(Boolean)
                    .join(" to ") || "Not set"}
                </TD>
                <TD className="text-right tabular-nums">
                  {c.budgetMinor === null ? (
                    <span className="text-muted-foreground">Not set</span>
                  ) : (
                    formatMoneyWhole(c.budgetMinor, c.currency)
                  )}
                </TD>
                <TD>
                  <span className="flex items-center gap-1.5" title={stagesLabel(c)}>
                    {c.stages.map((s) => (
                      <StatusMark key={s.id} status={asStatus(s.status)} className="size-2.5" />
                    ))}
                  </span>
                </TD>
                <TD>
                  <StatusChip status={state.status}>{state.label}</StatusChip>
                </TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </Panel>
  );
}

const asStatus = (s: string): Status =>
  s === "ready" || s === "caution" || s === "error" ? s : "pending";

const stagesLabel = (c: CampaignSummary) => c.stages.map((s) => `${s.id}: ${s.note}`).join(", ");

// CampaignsTab is a project's campaigns, with the form that starts one.
export function CampaignsTab({ projectId }: { projectId: number }) {
  const campaigns = useProjectCampaigns(projectId);
  const session = useSession();
  const list = campaigns.data?.campaigns ?? [];
  const cta = session.role !== "viewer" && (
    <Link to="/campaigns/new" search={{ project: projectId }} className={buttonVariants()}>
      <Plus /> New campaign
    </Link>
  );
  return (
    <div className="space-y-6">
      {campaigns.isPending && <LoadingRows rows={3} />}
      {campaigns.error && <ErrorNote error={campaigns.error} />}
      {campaigns.data && list.length === 0 && (
        <Panel>
          <EmptyState title="No campaigns yet">
            <span className="flex flex-col items-center gap-4">
              A campaign is one push: an offer, a window, a budget and channels. Its ads, its
              landing page and its plan are made inside it.
              {cta}
            </span>
          </EmptyState>
        </Panel>
      )}
      {list.length > 0 && <CampaignTable rows={list} />}
    </div>
  );
}

// NewCampaignPage starts a campaign: pick the project when the link didn't
// say, then the brief.
export function NewCampaignPage({ projectId: given }: { projectId?: number }) {
  const navigate = useNavigate();
  const projects = useProjects();
  const brands = useBrands();
  const plansList = usePlans();
  const create = useCreateCampaign();
  const [picked, setPicked] = useState<number | null>(given ?? null);
  const projectId = given ?? picked;
  const own = new Set(brands.data?.brands.filter((b) => b.kind === "own").map((b) => b.id) ?? []);
  const ours = (projects.data?.projects ?? []).filter(
    (p) => p.brandId !== null && own.has(p.brandId),
  );
  const project = ours.find((p) => p.id === projectId) ?? null;
  const brand = useBrand(project?.brandId ?? null);
  const blank: CampaignInput = {
    projectId: projectId ?? 0,
    name: "",
    goal: "",
    audience: "",
    avoid: "",
    offerId: 0,
    startsOn: "",
    endsOn: "",
    budgetMinor: null,
    currency: "AED",
    channels: ["meta_lead_form", "google_search"],
    markets: ["UAE"],
  };
  return (
    <>
      {project ? (
        <Link
          to="/projects/$projectId"
          params={{ projectId: String(project.id) }}
          search={{ tab: "campaigns" }}
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> {project.name}
        </Link>
      ) : (
        <Link
          to="/projects"
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Projects
        </Link>
      )}
      <PageHeader
        title="New campaign"
        subtitle="One push for a project: an offer, a window, a budget and channels. Adx writes the ads, drafts the page and plans from this brief."
        actions={
          given === undefined && (
            <Select
              aria-label="Project"
              placeholder="Project"
              value={projectId === null ? null : String(projectId)}
              onValueChange={(v) => setPicked(Number(v))}
              options={ours.map((p) => ({ value: String(p.id), label: p.name }))}
            />
          )
        }
      />
      {(projects.isPending || brands.isPending) && <LoadingRows rows={6} />}
      {projects.data && brands.data && projectId === null && (
        <Panel>
          <EmptyState title="Which project is this for?">
            Pick one above. A campaign always belongs to a development.
          </EmptyState>
        </Panel>
      )}
      {projectId !== null && project && (
        <BriefForm
          key={projectId}
          title="Brief"
          initial={blank}
          offers={(brand.data?.offers ?? []).filter(
            (o) => o.projectId === null || o.projectId === projectId,
          )}
          channels={plansList.data?.channels ?? []}
          pending={create.isPending}
          error={create.error}
          onCancel={() =>
            navigate({
              to: "/projects/$projectId",
              params: { projectId: String(projectId) },
              search: { tab: "campaigns" },
            })
          }
          onSubmit={(input) =>
            create.mutate(
              { ...input, projectId },
              {
                onSuccess: (c) =>
                  navigate({
                    to: "/projects/$projectId/campaigns/$campaignId",
                    params: { projectId: String(projectId), campaignId: String(c.id) },
                  }),
              },
            )
          }
        />
      )}
    </>
  );
}
