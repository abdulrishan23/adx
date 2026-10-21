import { Link } from "@tanstack/react-router";
import { Download, Upload } from "lucide-react";
import { useState } from "react";
import type { Ad } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { buttonVariants } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { ReviewTally } from "@/creative/ui";
import { AdTile, CommentsPanel, ReviewLinkButtons } from "../creative-project";
import { useAllCreative } from "./shared";

type Show = "all" | "revise" | "waiting" | "approved";

// Producer is every ad across campaigns, to adapt, approve and send on:
// Smartly's Producer. A campaign's Ads stage is the same ads for one
// campaign; here they're across all of them, worst first.
export function CreativeProducerPage() {
  const all = useAllCreative();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const [show, setShow] = useState<Show>("all");
  const [projectId, setProjectId] = useState<number>(0);
  const [campaignId, setCampaignId] = useState<number>(0);

  const keep = (a: Ad) => show === "all" || a.status === show;
  const everyAd = all.projects.flatMap((p) => p.concepts.flatMap((k) => k.ads));
  const count = (s: string) => everyAd.filter((a) => a.status === s).length;
  const projects = all.projects.filter(
    (p) => projectId === 0 || p.settings.projectId === projectId,
  );
  const campaigns = all.campaigns.filter((c) => projectId === 0 || c.projectId === projectId);

  return (
    <>
      <PageHeader
        title="Producer"
        subtitle="Every ad across campaigns: what needs changes, what's waiting for review, what's approved and ready to go out."
        actions={
          <>
            <Select
              aria-label="Project"
              value={String(projectId)}
              onValueChange={(v) => {
                setProjectId(Number(v));
                setCampaignId(0);
              }}
              options={[
                { value: "0", label: "All projects" },
                ...all.projects.map((p) => ({
                  value: String(p.settings.projectId),
                  label: p.settings.projectName,
                })),
              ]}
            />
            <Select
              aria-label="Campaign"
              value={String(campaignId)}
              onValueChange={(v) => setCampaignId(Number(v))}
              options={[
                { value: "0", label: "All campaigns" },
                ...campaigns.map((c) => ({ value: String(c.id), label: c.name })),
              ]}
            />
          </>
        }
      />
      <Segmented
        label="Show"
        value={show}
        onChange={setShow}
        options={[
          { value: "all", label: "All", count: everyAd.length },
          { value: "revise", label: "Needs changes", tone: "caution", count: count("revise") },
          { value: "waiting", label: "Awaiting review", tone: "pending", count: count("waiting") },
          { value: "approved", label: "Approved", tone: "ready", count: count("approved") },
        ]}
        className="mb-8"
      />
      {all.isPending && <LoadingRows rows={8} />}
      {all.error && <ErrorNote error={all.error} />}
      {!all.isPending && everyAd.length === 0 && (
        <Panel>
          <EmptyState title="No ads yet">
            Ads are made inside a campaign, from its concepts and the project's pictures.
          </EmptyState>
        </Panel>
      )}
      <div className="space-y-10">
        {projects.map((p) => {
          const pid = p.settings.projectId;
          const concepts = p.concepts
            .filter((k) => campaignId === 0 || k.campaignId === campaignId)
            .map((k) => ({ ...k, ads: k.ads.filter(keep) }))
            .filter((k) => k.ads.length > 0);
          if (concepts.length === 0) return null;
          const ads = p.concepts.flatMap((k) => k.ads);
          const n = (s: string) => ads.filter((a) => a.status === s).length;
          return (
            <section key={pid} className="space-y-5">
              <header className="flex flex-wrap items-center justify-between gap-4">
                <div className="space-y-1">
                  <h2 className="font-heading text-lg font-semibold">{p.settings.projectName}</h2>
                  <ReviewTally
                    approved={n("approved")}
                    revise={n("revise")}
                    waiting={n("waiting")}
                  />
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <ReviewLinkButtons settings={p.settings} />
                  {n("approved") > 0 && (
                    <a
                      href={`/api/creative/projects/${pid}/download?set=approved`}
                      className={buttonVariants({ variant: "outline" })}
                    >
                      <Download /> Approved
                    </a>
                  )}
                  {canEdit && (
                    <Link
                      to="/creative/$projectId/upload"
                      params={{ projectId: String(pid) }}
                      className={buttonVariants({ variant: "outline" })}
                    >
                      <Upload /> Upload ads
                    </Link>
                  )}
                </div>
              </header>
              {concepts.map((k) => (
                <div key={k.id} className="space-y-3">
                  <p className="flex flex-wrap items-baseline gap-x-3 text-sm">
                    <Link
                      to="/creative/$projectId/concepts/$conceptId"
                      params={{ projectId: String(pid), conceptId: String(k.id) }}
                      className="font-medium hover:underline"
                    >
                      {k.name}
                    </Link>
                    <span className="text-muted-foreground">
                      {all.campaignName(k.campaignId) ?? "No campaign"}
                    </span>
                  </p>
                  <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6">
                    {k.ads.map((a) => (
                      <AdTile key={a.id} ad={a} projectId={pid} canEdit={canEdit} />
                    ))}
                  </ul>
                </div>
              ))}
              {show !== "approved" && <CommentsPanel project={p} canEdit={canEdit} />}
            </section>
          );
        })}
      </div>
    </>
  );
}
