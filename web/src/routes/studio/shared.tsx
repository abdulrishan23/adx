import { useProjectsList } from "@/api/adx";
import { Select } from "@/components/ui/select";
import { StatusChip } from "@/components/ui/status-chip";

// BrandPicker chooses which brand a Create screen shows. Screens default to
// the first brand that has something, so a reader never lands on empty.
export function BrandPicker({
  value,
  onChange,
  only,
}: {
  value: number | null;
  onChange: (id: number) => void;
  // Brands that have data on this screen; the rest are listed as "not yet".
  only?: number[];
}) {
  const projects = useProjectsList();
  const options = (projects.data ?? []).map((p) => ({
    value: String(p.id),
    label: only && !only.includes(p.id) ? `${p.name} · not yet` : p.name,
  }));
  return (
    <Select
      aria-label="Brand"
      value={value === null ? null : String(value)}
      onValueChange={(v) => onChange(Number(v))}
      options={options}
      placeholder="Choose a brand"
    />
  );
}

// SampleChip marks a screen whose system hasn't shipped: everything on it is
// mocked, so nobody mistakes it for connected data.
// Without a phase it marks a screen drawn from sample data that has no phase
// of its own, such as Home before the accounts are connected.
export function SampleChip({ phase }: { phase?: 2 | 3 }) {
  return (
    <StatusChip
      status="pending"
      className="font-sans normal-case tracking-normal"
      title="Sample figures. Connected accounts replace them."
    >
      {phase ? `Sample · phase ${phase}` : "Sample data"}
    </StatusChip>
  );
}
