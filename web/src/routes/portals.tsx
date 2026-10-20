import { type FormEvent, useState } from "react";
import { usePortals, useSetPortalSpend } from "@/api/adx";
import type { PortalRow } from "@/api/models";
import { Form } from "@/components/form";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { SourceName } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDate, formatMoneyShort, formatNumber } from "@/lib/format";

const per = (minor: number | null, n: number) =>
  minor !== null && n > 0 ? Math.round(minor / n) : null;

export function PortalsPage() {
  const portals = usePortals();
  const month = portals.data?.[0]?.spendMonth;
  return (
    <>
      <PageHeader
        title="Portals"
        subtitle={
          month
            ? formatDate(`${month}-01T00:00:00Z`, { month: "long", year: "numeric" })
            : undefined
        }
      />
      {portals.isPending && <LoadingRows rows={4} />}
      {portals.error && <ErrorNote error={portals.error} />}
      {portals.data && (
        <Panel bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Portal</TH>
                <TH numeric>Leads</TH>
                <TH numeric>Qualified</TH>
                <TH numeric>Deals</TH>
                <TH numeric>Per lead</TH>
                <TH numeric>Per qualified</TH>
                <TH className="w-72">Spend this month</TH>
              </TR>
            </THead>
            <TBody>
              {portals.data.map((p) => (
                <TR key={p.key}>
                  <TD className="font-medium">
                    <SourceName name={p.name} />
                  </TD>
                  <TD numeric>{formatNumber(p.leads)}</TD>
                  <TD numeric>{formatNumber(p.qualified)}</TD>
                  <TD numeric>{p.deals}</TD>
                  <TD numeric>{formatMoneyShort(per(p.spendMinor, p.leads))}</TD>
                  <TD numeric>{formatMoneyShort(per(p.spendMinor, p.qualified))}</TD>
                  <TD>
                    <SpendField portal={p} />
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      )}
    </>
  );
}

function SpendField({ portal }: { portal: PortalRow }) {
  const save = useSetPortalSpend();
  const [editing, setEditing] = useState(portal.spendMinor === null);
  const [value, setValue] = useState(
    portal.spendMinor === null ? "" : String(portal.spendMinor / 100),
  );

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmed = value.replace(/,/g, "").trim();
    const spendMinor = trimmed === "" ? null : Math.round(Number(trimmed) * 100);
    save.mutate({ key: portal.key, spendMinor }, { onSuccess: () => setEditing(false) });
  }

  if (!editing) {
    return (
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono tabular-nums">{formatMoneyShort(portal.spendMinor)}</span>
        <Button size="xs" variant="ghost" onClick={() => setEditing(true)}>
          Edit
        </Button>
      </div>
    );
  }
  return (
    <Form onSubmit={onSubmit} className="flex items-center gap-2">
      <Input
        aria-label={`${portal.name} spend in AED`}
        inputMode="decimal"
        placeholder="AED this month"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="h-7 font-mono"
        aria-invalid={save.isError}
      />
      <Button size="sm" type="submit" disabled={save.isPending}>
        Save
      </Button>
      {save.error && <span className="text-xs text-destructive">{save.error.message}</span>}
    </Form>
  );
}
