import { type PageStatus, useBuiltPages } from "@/api/create";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatRelative } from "@/lib/format";
import { SampleChip } from "./shared";

const STATUS: Record<
  PageStatus,
  { chip: "ready" | "caution" | "pending" | "error"; label: string }
> = {
  live: { chip: "ready", label: "Live" },
  draft: { chip: "pending", label: "Draft" },
  broken: { chip: "error", label: "Broken" },
};

// Landing pages are built from approved facts, with their forms wired to
// GoHighLevel, so what the ad promises and what the page says agree.
export function PagesPage() {
  const pages = useBuiltPages();
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            Landing pages <SampleChip phase={3} />
          </span>
        }
        subtitle="Built from a brand's approved facts, one per language and offer. Each form lands in GoHighLevel under the right brand."
      />
      {pages.isPending && <LoadingRows rows={5} />}
      {pages.error && <ErrorNote error={pages.error} />}
      {pages.data && (
        <Panel bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Page</TH>
                <TH>Brand</TH>
                <TH>Status</TH>
                <TH>Language</TH>
                <TH numeric>Facts used</TH>
                <TH>Form</TH>
                <TH>Updated</TH>
              </TR>
            </THead>
            <TBody>
              {pages.data.map((p) => (
                <TR key={p.id}>
                  <TD>
                    <p
                      className="text-left font-medium"
                      dir={p.language === "ar" ? "rtl" : undefined}
                      lang={p.language === "ar" ? "ar" : undefined}
                    >
                      {p.title}
                    </p>
                    <p className="font-mono text-xs text-muted-foreground">{p.url}</p>
                  </TD>
                  <TD>{p.projectName}</TD>
                  <TD>
                    <StatusChip status={STATUS[p.status].chip}>{STATUS[p.status].label}</StatusChip>
                  </TD>
                  <TD>{p.language === "ar" ? "Arabic" : "English"}</TD>
                  <TD numeric>{p.factsUsed}</TD>
                  <TD className="text-muted-foreground">{p.form}</TD>
                  <TD className="text-muted-foreground">{formatRelative(p.updatedAt)}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      )}
    </>
  );
}
