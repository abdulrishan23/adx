import { Link } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useBrands } from "@/api/brands";
import type { BrandSummary } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { StatusMark } from "@/components/ui/status-chip";

// Brands is where Create starts. A brand is Reportage or a competitor; the
// developments it markets are its projects. Everything downstream, research,
// plans, creatives and pages, reads from a brand's approved facts and offers.
export function BrandsPage() {
  const brands = useBrands();
  const session = useSession();
  const own = brands.data?.brands.filter((b) => b.kind === "own") ?? [];
  const competitors = brands.data?.brands.filter((b) => b.kind === "competitor") ?? [];
  const unassigned = brands.data?.unassigned ?? [];
  return (
    <>
      <PageHeader
        title="Brands"
        subtitle="Who Adx markets and who it watches. A brand's projects, facts and offers are what every plan and creative is built from."
        actions={
          session.role === "admin" && (
            <Link to="/brands/new" className={buttonVariants()}>
              <Plus /> Start a brand
            </Link>
          )
        }
      />
      {brands.isPending && <LoadingRows rows={4} />}
      {brands.error && <ErrorNote error={brands.error} />}
      {brands.data && (
        <div className="space-y-8">
          {own.length === 0 ? (
            <Panel>
              <EmptyState title="No brand yet">
                Start one with its first project, and Adx begins the research.
              </EmptyState>
            </Panel>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {own.map((b, i) => (
                <BrandCard key={b.id} brand={b} delay={i * 40} />
              ))}
            </div>
          )}

          {unassigned.length > 0 && (
            <Panel
              title="Projects without a brand"
              description="Open a brand to attach them, so their facts and offers have a home."
              bodyClassName="p-0"
            >
              <ul className="divide-y">
                {unassigned.map((p) => (
                  <li key={p.id} className="px-5 py-2.5 text-sm">
                    {p.name}
                  </li>
                ))}
              </ul>
            </Panel>
          )}

          {competitors.length > 0 && (
            <section>
              <h2 className="mb-3 font-mono text-[0.65rem] tracking-widest text-muted-foreground uppercase">
                Competitors Adx watches
              </h2>
              <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                {competitors.map((b, i) => (
                  <BrandCard key={b.id} brand={b} delay={i * 40} />
                ))}
              </div>
            </section>
          )}
        </div>
      )}
    </>
  );
}

function BrandCard({ brand: b, delay }: { brand: BrandSummary; delay: number }) {
  const status = b.openConflicts > 0 ? "caution" : b.projectCount > 0 ? "ready" : "pending";
  return (
    <Link
      to="/brands/$brandId"
      params={{ brandId: String(b.id) }}
      className="flex animate-rise flex-col border bg-card p-5 transition-colors hover:border-foreground/40"
      style={{ animationDelay: `${delay}ms` }}
    >
      <p className="flex items-center gap-1.5 font-mono text-[0.6rem] tracking-widest text-muted-foreground uppercase">
        <StatusMark status={status} />
        {b.kind === "own" ? "Ours" : "Competitor"}
      </p>
      <h2 className="mt-1.5 font-heading text-xl font-semibold tracking-tight">{b.name}</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        {b.projectCount === 0
          ? "No projects yet"
          : `${b.projectCount} ${b.projectCount === 1 ? "project" : "projects"}`}
        {b.openConflicts > 0 &&
          ` · ${b.openConflicts} ${b.openConflicts === 1 ? "fact" : "facts"} to decide`}
      </p>
      {b.notes && <p className="mt-3 line-clamp-2 text-sm text-muted-foreground">{b.notes}</p>}
    </Link>
  );
}
