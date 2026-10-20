import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Select } from "@/components/ui/select";
import { StatusMark } from "@/components/ui/status-chip";
import { ReviewTally } from "@/creative/ui";
import { AskForConcepts } from "../creative-project";
import { useAllCreative } from "./shared";

// Concepts is every idea across projects, each tagged with its campaign: the
// first of Creative's apps, as in Smartly. Ask Adx writes new ones here for a
// project and, optionally, a campaign.
export function CreativeConceptsPage() {
  const all = useAllCreative();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const first = all.projects[0]?.settings.projectId ?? null;
  const [projectId, setProjectId] = useState<number | null>(null);
  const [campaignId, setCampaignId] = useState<number>(0);
  const target = projectId ?? first;
  const projectCampaigns = all.campaigns.filter((c) => c.projectId === target);

  return (
    <>
      <PageHeader
        title="Concepts"
        subtitle="Every idea across projects and the ads built from it. Adx writes them from the brief and the approved facts."
      />
      {canEdit && target !== null && (
        <Panel className="mb-6">
          <div className="flex flex-wrap items-end gap-x-3 gap-y-4">
            <Select
              aria-label="Project"
              value={String(target)}
              onValueChange={(v) => {
                setProjectId(Number(v));
                setCampaignId(0);
              }}
              options={all.projects.map((p) => ({
                value: String(p.settings.projectId),
                label: p.settings.projectName,
              }))}
            />
            <Select
              aria-label="Campaign"
              value={String(campaignId)}
              onValueChange={(v) => setCampaignId(Number(v))}
              options={[
                { value: "0", label: "No campaign" },
                ...projectCampaigns.map((c) => ({ value: String(c.id), label: c.name })),
              ]}
            />
            <AskForConcepts
              key={`${target}-${campaignId}`}
              projectId={target}
              campaignId={campaignId}
            />
          </div>
        </Panel>
      )}
      {all.isPending && <LoadingRows rows={6} />}
      {all.error && <ErrorNote error={all.error} />}
      {!all.isPending && all.projects.every((p) => p.concepts.length === 0) && (
        <Panel>
          <EmptyState title="No concepts yet">
            Ask Adx above, or open a campaign and write them from its brief.
          </EmptyState>
        </Panel>
      )}
      <div className="space-y-8">
        {all.projects
          .filter((p) => p.concepts.length > 0)
          .map((p) => (
            <section key={p.settings.projectId}>
              <h2 className="mb-3 font-heading text-lg font-semibold">{p.settings.projectName}</h2>
              <Panel bodyClassName="p-0">
                <ul className="divide-y">
                  {p.concepts.map((k) => {
                    const ads = k.ads;
                    const count = (s: string) => ads.filter((a) => a.status === s).length;
                    const campaign = all.campaignName(k.campaignId);
                    return (
                      <li key={k.id}>
                        <Link
                          to="/creative/$projectId/concepts/$conceptId"
                          params={{
                            projectId: String(p.settings.projectId),
                            conceptId: String(k.id),
                          }}
                          className="flex flex-wrap items-center gap-x-6 gap-y-2 px-6 py-4 text-sm hover:bg-muted"
                        >
                          <StatusMark
                            status={
                              count("revise") > 0
                                ? "caution"
                                : ads.length > 0 && count("approved") === ads.length
                                  ? "ready"
                                  : "pending"
                            }
                          />
                          <span className="min-w-0 flex-1">
                            <span className="font-medium">{k.name}</span>
                            {k.why && (
                              <span className="block truncate text-muted-foreground">{k.why}</span>
                            )}
                          </span>
                          <span className="w-40 truncate text-muted-foreground">
                            {campaign ?? "No campaign"}
                          </span>
                          <span className="w-16 text-right text-muted-foreground tabular-nums">
                            {ads.length} {ads.length === 1 ? "ad" : "ads"}
                          </span>
                          <span className="w-56">
                            {ads.length > 0 && (
                              <ReviewTally
                                approved={count("approved")}
                                revise={count("revise")}
                                waiting={count("waiting")}
                              />
                            )}
                          </span>
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </Panel>
            </section>
          ))}
      </div>
    </>
  );
}
