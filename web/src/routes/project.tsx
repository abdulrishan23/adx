import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { useBrand } from "@/api/brands";
import { useCreativeOverview } from "@/api/creative";
import { usePages } from "@/api/pages";
import { usePlans } from "@/api/plans";
import { useBriefs } from "@/api/research";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";
import { CLASS, factValue } from "./brand";

const route = getRouteApi("/app/brands/$brandId/projects/$projectId");

// A project is one development under a brand, such as The Wadi under
// Reportage. Its page gathers what Studio holds for it: the facts Adx may
// state, its offers, the creatives, landing pages, plans and research.
export function ProjectPage() {
  const { brandId, projectId } = route.useParams();
  const bid = Number(brandId);
  const pid = Number(projectId);
  const brand = useBrand(bid);
  const creative = useCreativeOverview();
  const pages = usePages();
  const plans = usePlans();
  const briefs = useBriefs();
  if (brand.isPending) return <LoadingRows rows={8} />;
  if (brand.error) return <ErrorNote error={brand.error} />;
  if (!brand.data) return <LoadingRows rows={8} />;
  const d = brand.data;
  const project = d.projects.find((p) => p.id === pid);
  if (!project) {
    return (
      <Panel>
        <EmptyState title="Not one of this brand's projects" />
      </Panel>
    );
  }
  const facts = d.facts.filter((f) => f.projectId === pid || f.projectId === null);
  const offers = d.offers.filter((o) => o.projectId === pid || o.projectId === null);
  const studio = creative.data?.projects.find((p) => p.projectId === pid);
  const projectPages = (pages.data?.pages ?? []).filter((p) => p.projectId === pid);
  const projectPlans = (plans.data?.plans ?? []).filter((p) => p.projectId === pid);
  const projectBriefs = (briefs.data?.briefs ?? []).filter((b) => b.projectId === pid);
  const toReview = facts.filter((f) => f.class === "needs_review").length;

  return (
    <>
      <Link
        to="/brands/$brandId"
        params={{ brandId }}
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> {d.brand.name}
      </Link>
      <PageHeader
        title={project.name}
        subtitle={`A ${d.brand.name} development. ${facts.filter((f) => f.class === "approved").length} approved facts${toReview ? `, ${toReview} to review` : ""}, ${offers.length} ${offers.length === 1 ? "offer" : "offers"}.`}
        actions={
          <Link
            to="/creative/$projectId"
            params={{ projectId }}
            className={buttonVariants({ size: "sm" })}
          >
            Open in Creatives
          </Link>
        }
      />
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="space-y-6">
          <Panel
            title="What Adx may say"
            description="Approved facts are the only figures that reach ads and pages. Brand-wide facts apply here too."
            bodyClassName="p-0"
          >
            {facts.length === 0 ? (
              <EmptyState title="No facts yet">Record them on the brand page.</EmptyState>
            ) : (
              <ul className="divide-y">
                {facts.map((f) => (
                  <li key={f.id} className="flex items-start gap-4 px-6 py-4">
                    <StatusMark status={CLASS[f.class].status} className="mt-1.5" />
                    <div className="min-w-0 flex-1">
                      <p className="font-mono text-xs text-muted-foreground">{f.key}</p>
                      <p className="mt-0.5">{factValue(f)}</p>
                    </div>
                    {f.class !== "approved" && (
                      <span className="text-xs text-muted-foreground">{CLASS[f.class].label}</span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          <Panel title="Offers" bodyClassName="p-0">
            {offers.length === 0 ? (
              <EmptyState title="No offers" />
            ) : (
              <ul className="divide-y">
                {offers.map((o) => (
                  <li key={o.id} className="flex items-start gap-4 px-6 py-4">
                    <StatusMark
                      status={o.current.state === "approved" ? "ready" : "pending"}
                      className="mt-1.5"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="font-medium">{o.name}</p>
                      <p className="mt-0.5 text-sm text-muted-foreground">
                        {o.current.document.headline}
                      </p>
                      {o.current.document.headlineAr && (
                        <p
                          className="mt-0.5 text-left text-sm text-muted-foreground"
                          dir="rtl"
                          lang="ar"
                        >
                          {o.current.document.headlineAr}
                        </p>
                      )}
                    </div>
                    <span className="text-xs text-muted-foreground capitalize">
                      {o.current.state}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>

        <div className="space-y-6">
          <Section
            title="Creatives"
            to={{ kind: "creative", projectId }}
            empty={!studio ? "Not set up in Creatives yet." : undefined}
            rows={
              studio
                ? [
                    {
                      label: `${studio.concepts} ${studio.concepts === 1 ? "concept" : "concepts"} · ${studio.ads} ${studio.ads === 1 ? "ad" : "ads"}`,
                      status: "pending",
                    },
                    { label: `${studio.approved} approved`, status: "ready" },
                    ...(studio.revise
                      ? [{ label: `${studio.revise} need changes`, status: "caution" as Status }]
                      : []),
                    ...(studio.waiting
                      ? [
                          {
                            label: `${studio.waiting} waiting for review`,
                            status: "pending" as Status,
                          },
                        ]
                      : []),
                  ]
                : []
            }
          />
          <Section
            title="Landing pages"
            to={{ kind: "pages" }}
            empty={projectPages.length === 0 ? "No pages yet." : undefined}
            rows={projectPages.map((p) => ({
              label: p.name,
              status: p.publishedVersion === null ? "pending" : "ready",
              note: p.publishedVersion === null ? "Draft" : `Live v${p.publishedVersion}`,
              link: { kind: "page", id: p.id },
            }))}
          />
          <Section
            title="Media plans"
            to={{ kind: "plans" }}
            empty={projectPlans.length === 0 ? "No plans yet." : undefined}
            rows={projectPlans.map((p) => ({
              label: p.title,
              status: p.accepted ? "ready" : "pending",
              note: `v${p.latestVersion}${p.accepted ? " · accepted" : ""}`,
              link: { kind: "plan", id: p.id },
            }))}
          />
          <Section
            title="Research"
            to={{ kind: "research" }}
            empty={projectBriefs.length === 0 ? "No research yet." : undefined}
            rows={projectBriefs.map((b) => ({
              label: b.title,
              status: "pending",
              note: formatRelative(b.createdAt),
            }))}
          />
        </div>
      </div>
    </>
  );
}

type Row = {
  label: string;
  status: Status;
  note?: string;
  link?: { kind: "page" | "plan"; id: number };
};

type Target =
  | { kind: "creative"; projectId: string }
  | { kind: "pages" }
  | { kind: "plans" }
  | { kind: "research" };

function TargetLink({ to, children }: { to: Target; children: React.ReactNode }) {
  const cls = "text-xs text-muted-foreground hover:text-foreground";
  switch (to.kind) {
    case "creative":
      return (
        <Link to="/creative/$projectId" params={{ projectId: to.projectId }} className={cls}>
          {children}
        </Link>
      );
    case "pages":
      return (
        <Link to="/studio/pages" className={cls}>
          {children}
        </Link>
      );
    case "plans":
      return (
        <Link to="/studio/plans" className={cls}>
          {children}
        </Link>
      );
    case "research":
      return (
        <Link to="/studio/research" className={cls}>
          {children}
        </Link>
      );
  }
}

function Section({
  title,
  to,
  rows,
  empty,
}: {
  title: string;
  to: Target;
  rows: Row[];
  empty?: string;
}) {
  return (
    <Panel title={title} action={<TargetLink to={to}>Open</TargetLink>} bodyClassName="p-0">
      {empty ? (
        <p className="px-6 py-5 text-sm text-muted-foreground">{empty}</p>
      ) : (
        <ul className="divide-y">
          {rows.map((r) => (
            <li key={r.label} className="flex items-center gap-3 px-6 py-3.5 text-sm">
              <StatusMark status={r.status} />
              <span className="min-w-0 flex-1 truncate">
                {r.link?.kind === "page" ? (
                  <Link
                    to="/studio/pages/$pageId"
                    params={{ pageId: String(r.link.id) }}
                    className="hover:underline"
                  >
                    {r.label}
                  </Link>
                ) : r.link?.kind === "plan" ? (
                  <Link
                    to="/studio/plans/$planId"
                    params={{ planId: String(r.link.id) }}
                    className="hover:underline"
                  >
                    {r.label}
                  </Link>
                ) : (
                  r.label
                )}
              </span>
              {r.note && <span className="text-xs text-muted-foreground">{r.note}</span>}
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
