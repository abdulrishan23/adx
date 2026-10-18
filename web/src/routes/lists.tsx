import { useState } from "react";
import { useLists } from "@/api/adx";
import type { CatalogueEntry } from "@/api/models";
import { useStageOverview } from "@/api/stages";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { released } from "@/lib/release";
import { StageTable } from "./stage-table";

type Tab = "stages" | "aliases" | "pages";

export function ListsPage() {
  const lists = useLists();
  const stages = useStageOverview();
  const [tab, setTab] = useState<Tab>("stages");
  const unmapped = stages.data?.statuses.filter((st) => st.currentStage === null).length ?? 0;
  const unconfirmed = lists.data?.catalogue.filter((c) => !c.confirmed).length ?? 0;
  const pages = released(4);

  return (
    <>
      <PageHeader
        title="Stages and lists"
        subtitle="How GoHighLevel's lead statuses count in Adx's funnel, and the names Adx matches campaigns by."
      />
      <Segmented
        className="mb-4"
        label="List"
        value={tab}
        onChange={setTab}
        options={[
          { value: "stages", label: "Statuses", count: unmapped },
          { value: "aliases", label: "Project names" },
          ...(pages
            ? [{ value: "pages" as const, label: "Website pages", count: unconfirmed }]
            : []),
        ]}
      />
      {tab === "stages" && <StageTable />}
      {tab === "aliases" && (
        <Panel>
          <EmptyState title="Not set up yet">
            Adx reads project names from campaign names once Google Ads and Meta are connected.
            Until then there are no names to match.
          </EmptyState>
        </Panel>
      )}
      {pages && tab === "pages" && lists.isPending && <LoadingRows rows={8} />}
      {pages && tab === "pages" && lists.error && <ErrorNote error={lists.error} />}
      {pages && tab === "pages" && lists.data && <Pages catalogue={lists.data.catalogue} />}
    </>
  );
}

const WEIGHT_LABEL = ["Context only", "Interest", "", "Strong intent"];

function Pages({ catalogue }: { catalogue: CatalogueEntry[] }) {
  const rows = [...catalogue].sort((a, b) => Number(a.confirmed) - Number(b.confirmed));
  return (
    <Panel title="Website page catalogue" bodyClassName="p-0">
      <Table>
        <THead>
          <TR>
            <TH>Page</TH>
            <TH>Project</TH>
            <TH>Type</TH>
            <TH>Counts as</TH>
            <TH />
          </TR>
        </THead>
        <TBody>
          {rows.map((c) => (
            <TR key={c.url}>
              <TD className="max-w-80 truncate font-mono text-xs">{c.url}</TD>
              <TD>{c.project ?? <span className="text-muted-foreground">Any</span>}</TD>
              <TD>{c.pageType}</TD>
              <TD className="text-muted-foreground">
                {WEIGHT_LABEL[c.weight] || `Weight ${c.weight}`}
              </TD>
              <TD className="text-right">
                {c.confirmed ? (
                  <StatusChip status="ready">Confirmed</StatusChip>
                ) : (
                  <StatusChip status="pending">To review</StatusChip>
                )}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </Panel>
  );
}
