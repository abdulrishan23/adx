import { Popover } from "@base-ui/react/popover";
import { ChevronDown } from "lucide-react";
import { useState } from "react";
import type { AnalyticsSearch } from "@/lib/analytics";
import { Button } from "./ui/button";

type Selection = AnalyticsSearch["project"];

export function ProjectFilter({
  value,
  projects,
  onChange,
}: {
  value: Selection;
  projects: { id: number; name: string }[];
  onChange: (value: Selection) => void;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Selection>(value);
  const selected = value === "all" ? [] : Array.isArray(value) ? value : [value];
  const label =
    value === "all"
      ? "All projects"
      : selected.length === 1
        ? (projects.find((p) => p.id === selected[0])?.name ?? `Project ${selected[0]}`)
        : `${selected.length} projects`;
  const ids = draft === "all" ? projects.map((p) => p.id) : Array.isArray(draft) ? draft : [draft];
  const all = draft === "all" || (projects.length > 0 && projects.every((p) => ids.includes(p.id)));
  function toggle(id: number) {
    setDraft(ids.includes(id) ? ids.filter((item) => item !== id) : [...ids, id]);
  }
  return (
    <Popover.Root
      open={open}
      onOpenChange={(next) => {
        if (next) setDraft(value);
        setOpen(next);
      }}
    >
      <Popover.Trigger
        aria-label={`Projects: ${label}`}
        className="flex h-10 items-center gap-2 border bg-card px-3 text-sm outline-none hover:border-ring focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="max-w-52 truncate">{label}</span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner align="end" sideOffset={8} collisionPadding={12} className="z-50">
          <Popover.Popup className="max-h-(--available-height) w-72 overflow-y-auto max-w-[calc(100vw-24px)] border bg-popover text-popover-foreground shadow-xl outline-none">
            <Popover.Title className="px-4 pt-4 text-sm font-medium">Projects</Popover.Title>
            <label className="mx-2 mt-2 flex cursor-pointer items-center gap-3 border-b px-2 py-3 text-sm hover:bg-muted">
              <input
                type="checkbox"
                checked={all}
                ref={(el) => {
                  if (el) el.indeterminate = !all && ids.length > 0;
                }}
                onChange={() => setDraft(all ? [] : "all")}
              />
              All projects
            </label>
            <div className="max-h-72 overflow-y-auto p-2">
              {projects.map((project) => (
                <label
                  key={project.id}
                  className="flex cursor-pointer items-center gap-3 px-2 py-3 text-sm hover:bg-muted"
                >
                  <input
                    type="checkbox"
                    checked={ids.includes(project.id)}
                    onChange={() => toggle(project.id)}
                  />
                  {project.name}
                </label>
              ))}
              {projects.length === 0 && (
                <p className="px-2 py-3 text-sm text-muted-foreground">Loading projects…</p>
              )}
            </div>
            <div className="flex items-center justify-end gap-2 border-t p-3">
              <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button
                size="sm"
                disabled={ids.length === 0}
                onClick={() => {
                  onChange(
                    all
                      ? "all"
                      : ids.length === 1
                        ? (ids[0] ?? "all")
                        : [...ids].sort((a, b) => a - b),
                  );
                  setOpen(false);
                }}
              >
                Apply
              </Button>
            </div>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
