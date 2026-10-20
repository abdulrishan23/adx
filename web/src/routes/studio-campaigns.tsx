import { Link, useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useState } from "react";
import { useBrand } from "@/api/brands";
import { useCreateCampaign, useProjectCampaigns } from "@/api/campaigns";
import { usePlans } from "@/api/plans";
import type { CampaignInput, CampaignSummary } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
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
export function CampaignsTab({
  projectId,
  brandId,
}: {
  projectId: number;
  brandId: number | null;
}) {
  const campaigns = useProjectCampaigns(projectId);
  const brand = useBrand(brandId);
  const session = useSession();
  const navigate = useNavigate();
  const create = useCreateCampaign();
  const plansList = usePlans();
  const [starting, setStarting] = useState(false);
  const list = campaigns.data?.campaigns ?? [];
  const blank: CampaignInput = {
    projectId,
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
    <div className="space-y-6">
      {session.role !== "viewer" && !starting && (
        <div className="flex justify-end">
          <Button onClick={() => setStarting(true)}>
            <Plus /> New campaign
          </Button>
        </div>
      )}
      {starting && (
        <BriefForm
          title="New campaign"
          initial={blank}
          offers={(brand.data?.offers ?? []).filter(
            (o) => o.projectId === null || o.projectId === projectId,
          )}
          channels={plansList.data?.channels ?? []}
          pending={create.isPending}
          error={create.error}
          onCancel={() => setStarting(false)}
          onSubmit={(input) =>
            create.mutate(input, {
              onSuccess: (c) =>
                navigate({
                  to: "/projects/$projectId/campaigns/$campaignId",
                  params: { projectId: String(projectId), campaignId: String(c.id) },
                }),
            })
          }
        />
      )}
      {campaigns.isPending && <LoadingRows rows={3} />}
      {campaigns.error && <ErrorNote error={campaigns.error} />}
      {campaigns.data && list.length === 0 && !starting && (
        <Panel>
          <EmptyState title="No campaigns yet">
            <span className="flex flex-col items-center gap-4">
              A campaign is one push: an offer, a window, a budget and channels. Its ads, its
              landing page and its plan are made inside it.
              {session.role !== "viewer" && (
                <Button onClick={() => setStarting(true)}>
                  <Plus /> New campaign
                </Button>
              )}
            </span>
          </EmptyState>
        </Panel>
      )}
      {list.length > 0 && <CampaignTable rows={list} />}
    </div>
  );
}
