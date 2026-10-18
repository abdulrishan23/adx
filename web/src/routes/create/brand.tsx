import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { type Fact, type FactStatus, useApproveFact, useBrandKnowledge } from "@/api/create";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button, buttonVariants } from "@/components/ui/button";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";
import { BrandPicker, SampleChip } from "./shared";

const BRANDS_WITH_KNOWLEDGE = [1, 4, 3];

const STATUS: Record<
  FactStatus,
  { chip: "ready" | "caution" | "pending" | "error"; label: string }
> = {
  approved: { chip: "ready", label: "Approved" },
  conflict: { chip: "caution", label: "Conflict" },
  proposed: { chip: "pending", label: "Proposed" },
  outdated: { chip: "pending", label: "Outdated" },
};

// Brand knowledge is the approved facts about a brand, each with its source.
// Research, plans, creatives and pages read from here, so a fact in conflict
// blocks whatever depends on it until someone decides.
export function BrandKnowledgePage() {
  const [brandId, setBrandId] = useState<number>(1);
  const brand = useBrandKnowledge(brandId);
  const approve = useApproveFact();
  const b = brand.data;
  const conflicts = b ? b.facts.filter((f) => f.status === "conflict") : [];
  const groups = b
    ? [...new Set(b.facts.map((f) => f.topic))].map((topic) => ({
        topic,
        facts: b.facts.filter((f) => f.topic === topic && f.status !== "conflict"),
      }))
    : [];

  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            Brand knowledge <SampleChip phase={2} />
          </span>
        }
        subtitle="What Adx knows about a brand, with where each fact came from. Everything downstream reads from the approved facts."
        actions={<BrandPicker value={brandId} onChange={setBrandId} only={BRANDS_WITH_KNOWLEDGE} />}
      />
      {brand.isPending && <LoadingRows rows={8} />}
      {brand.error && <ErrorNote error={brand.error} />}
      {brand.data === null && (
        <Panel>
          <EmptyState title="Nothing captured for this brand yet">
            Adx captures facts from the website, brochures, enquiry forms and sales notes once the
            brand is set up. That arrives with phase 2.
          </EmptyState>
        </Panel>
      )}
      {b && (
        <div className="space-y-5">
          {conflicts.length > 0 && (
            <Panel
              title={
                groupConflicts(conflicts).length === 1
                  ? "A fact to decide"
                  : `${groupConflicts(conflicts).length} facts to decide`
              }
              description="Two sources disagree. Research and plans can't use this fact until one is approved."
              className="border-status-caution/50"
            >
              {groupConflicts(conflicts).map((pair) => (
                <div key={pair.key} className="grid gap-3 md:grid-cols-2">
                  {pair.facts.map((f) => (
                    <div key={f.id} className="flex flex-col gap-3 border p-4">
                      <Eyebrow>{f.topic}</Eyebrow>
                      <p className="font-medium">{f.text}</p>
                      <Source fact={f} />
                      <Button
                        size="sm"
                        variant="outline"
                        className="self-start"
                        disabled={approve.isPending}
                        onClick={() => approve.mutate({ projectId: b.projectId, factId: f.id })}
                      >
                        Use this one
                      </Button>
                    </div>
                  ))}
                </div>
              ))}
            </Panel>
          )}

          <div className="grid gap-5 lg:grid-cols-[2fr_1fr]">
            <div className="space-y-5">
              {groups.map((g) =>
                g.facts.length === 0 ? null : (
                  <Panel key={g.topic} title={g.topic} bodyClassName="p-0">
                    <ul className="divide-y">
                      {g.facts.map((f) => (
                        <li key={f.id} className="flex items-start gap-4 px-5 py-3">
                          <div className="min-w-0 flex-1">
                            <p
                              className={
                                f.status === "outdated" ? "text-muted-foreground line-through" : ""
                              }
                            >
                              {f.text}
                            </p>
                            <Source fact={f} />
                          </div>
                          <StatusChip status={STATUS[f.status].chip}>
                            {STATUS[f.status].label}
                          </StatusChip>
                          {f.status === "proposed" && (
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() =>
                                approve.mutate({ projectId: b.projectId, factId: f.id })
                              }
                            >
                              Approve
                            </Button>
                          )}
                        </li>
                      ))}
                    </ul>
                  </Panel>
                ),
              )}
            </div>
            <div className="space-y-5">
              <Panel title="Captured from" description={`Updated ${formatRelative(b.updatedAt)}`}>
                <ul className="space-y-2 text-sm">
                  {b.capturedFrom.map((c) => (
                    <li key={c.label} className="flex items-baseline justify-between gap-3">
                      <span className="min-w-0 truncate">{c.label}</span>
                      <span className="shrink-0 font-mono text-xs text-muted-foreground">
                        {c.items} facts
                      </span>
                    </li>
                  ))}
                </ul>
              </Panel>
              <Panel
                title="Brand kit"
                description="Logos, fonts and colours live with the creatives."
              >
                <Link
                  to="/creative/$projectId"
                  params={{ projectId: String(b.projectId) }}
                  search={{ tab: "brand" }}
                  className={buttonVariants({ variant: "outline", size: "sm" })}
                >
                  Open brand kit
                </Link>
              </Panel>
              <Panel title="Who reads this">
                <ul className="space-y-1.5 text-sm text-muted-foreground">
                  <li className="flex items-center gap-2">
                    <StatusMark status="ready" /> Research, for positioning and keywords
                  </li>
                  <li className="flex items-center gap-2">
                    <StatusMark status="ready" /> Media plans, for audiences and offers
                  </li>
                  <li className="flex items-center gap-2">
                    <StatusMark status="ready" /> Creatives and pages, for copy that's true
                  </li>
                  <li className="flex items-center gap-2">
                    <StatusMark status="ready" /> Ask Adx and the voice agent, for answers
                  </li>
                </ul>
              </Panel>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function Source({ fact }: { fact: Fact }) {
  const KIND: Record<Fact["source"]["kind"], string> = {
    website: "Website",
    brochure: "Brochure",
    crm: "GoHighLevel",
    sales: "Sales",
    forms: "Enquiry forms",
  };
  return (
    <p className="mt-0.5 text-xs text-muted-foreground">
      {KIND[fact.source.kind]} · {fact.source.label} · {formatRelative(fact.source.seenAt)}
    </p>
  );
}

function groupConflicts(facts: Fact[]) {
  const keys = [...new Set(facts.map((f) => f.conflict ?? f.id))];
  return keys.map((key) => ({ key, facts: facts.filter((f) => (f.conflict ?? f.id) === key) }));
}
