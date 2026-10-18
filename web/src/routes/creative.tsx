import { Link, useNavigate } from "@tanstack/react-router";
import { Palette } from "lucide-react";
import { useState } from "react";
import { useCreativeOverview, useSetUpCreative } from "@/api/creative";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { ReviewTally } from "@/creative/ui";
import { formatDate } from "@/lib/format";

// Creatives lists each project's concepts, ads and where their review stands.
export function CreativePage() {
  const overview = useCreativeOverview();
  const session = useSession();
  const canEdit = session.role !== "viewer";

  return (
    <section>
      <PageHeader
        title="Creatives"
        subtitle="Ads for each project in four sizes, reviewed on a link anyone in the review group can open."
        actions={canEdit && overview.data && <SetUp notSetUp={overview.data.notSetUp} />}
      />
      {overview.isPending && <LoadingRows rows={4} />}
      {overview.error && <ErrorNote error={overview.error} />}
      {overview.data &&
        (overview.data.projects.length === 0 ? (
          <Panel>
            <EmptyState title="No brand has creatives yet" icon={<Palette />}>
              {overview.data.notSetUp.length === 0
                ? "Add a brand first. Creatives belong to a brand."
                : canEdit
                  ? "Set up creatives for a brand to upload ads or build them from copy."
                  : "Someone on the marketing team sets them up."}
            </EmptyState>
          </Panel>
        ) : (
          <Panel bodyClassName="p-0">
            <Table>
              <THead>
                <TR>
                  <TH>Brand</TH>
                  <TH className="text-right">Concepts</TH>
                  <TH className="text-right">Ads</TH>
                  <TH>Review</TH>
                  <TH className="text-right">Renders</TH>
                  <TH>Review link</TH>
                  <TH className="text-right">Updated</TH>
                </TR>
              </THead>
              <TBody>
                {overview.data.projects.map((p) => (
                  <TR key={p.projectId}>
                    <TD>
                      <Link
                        to="/creative/$projectId"
                        params={{ projectId: String(p.projectId) }}
                        className="font-medium hover:underline"
                      >
                        {p.name}
                      </Link>
                      <div className="font-mono text-xs text-muted-foreground">{p.prefix}</div>
                    </TD>
                    <TD className="text-right font-mono tabular-nums">{p.concepts}</TD>
                    <TD className="text-right font-mono tabular-nums">
                      {p.ads}
                      {p.unpublished > 0 && (
                        <div className="text-xs text-muted-foreground">
                          {p.unpublished} layout only
                        </div>
                      )}
                    </TD>
                    <TD>
                      <ReviewTally approved={p.approved} revise={p.revise} waiting={p.waiting} />
                    </TD>
                    <TD className="text-right font-mono tabular-nums">{p.renders}</TD>
                    <TD className="text-sm">{p.reviewActive ? "Open" : "Paused"}</TD>
                    <TD className="text-right text-sm text-muted-foreground">
                      {formatDate(p.updatedAt)}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </Panel>
        ))}
    </section>
  );
}

function SetUp({ notSetUp }: { notSetUp: { projectId: number; name: string }[] }) {
  const [projectId, setProjectId] = useState<string | null>(null);
  const setUp = useSetUpCreative();
  const navigate = useNavigate();
  if (notSetUp.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        aria-label="Project"
        placeholder="Choose a brand"
        value={projectId}
        onValueChange={setProjectId}
        options={notSetUp.map((p) => ({ value: String(p.projectId), label: p.name }))}
        className="w-56"
      />
      <Button
        disabled={!projectId || setUp.isPending}
        onClick={() => {
          if (!projectId) return;
          setUp.mutate(Number(projectId), {
            onSuccess: (s) =>
              navigate({
                to: "/creative/$projectId",
                params: { projectId: String(s.projectId) },
              }),
          });
        }}
      >
        {setUp.isPending ? "Setting up…" : "Set up creatives"}
      </Button>
      {setUp.error && <p className="w-full text-sm text-destructive">{setUp.error.message}</p>}
    </div>
  );
}
