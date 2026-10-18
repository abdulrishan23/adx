import { Select as SelectPrimitive } from "@base-ui/react/select";
import { Check, ChevronDown } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export type SelectOption<V extends string> = {
  value: V;
  label: string;
  disabled?: boolean;
  // A small mark before the label, such as a platform's colour.
  icon?: ReactNode;
};

// Select is the one dropdown in Adx. Values are strings, the way the DOM
// keeps them, so callers convert numbers at the edge.
export function Select<V extends string>({
  value,
  onValueChange,
  options,
  placeholder,
  disabled,
  className,
  "aria-label": ariaLabel,
}: {
  value: V | null;
  onValueChange: (value: V) => void;
  options: SelectOption<V>[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
}) {
  // When some options carry an icon, the rest get an empty slot its size, so
  // every label starts at the same place.
  const icons = options.some((o) => o.icon);
  const lead = (o: SelectOption<V>) =>
    icons
      ? (o.icon ?? <span aria-hidden="true" className="inline-block size-2.5 shrink-0" />)
      : null;
  return (
    <SelectPrimitive.Root
      value={value}
      onValueChange={(v) => {
        if (v !== null) onValueChange(v as V);
      }}
      items={options.map((o) => ({ value: o.value, label: o.label }))}
      disabled={disabled}
    >
      <SelectPrimitive.Trigger
        aria-label={ariaLabel}
        className={cn(
          "flex h-8 min-w-0 items-center justify-between gap-2 border border-input bg-card pr-2 pl-2.5 text-left text-sm text-foreground whitespace-nowrap transition-colors outline-none hover:border-ring focus-visible:border-foreground focus-visible:ring-3 focus-visible:ring-ring/40 disabled:cursor-not-allowed disabled:opacity-50 data-popup-open:border-foreground",
          className,
        )}
      >
        <SelectPrimitive.Value className="flex min-w-0 items-center gap-2 truncate">
          {(v: V | null) => {
            const o = options.find((x) => x.value === v);
            if (!o) return <span className="text-muted-foreground">{placeholder}</span>;
            return (
              <>
                {o.icon}
                <span className="truncate">{o.label}</span>
              </>
            );
          }}
        </SelectPrimitive.Value>
        <SelectPrimitive.Icon className="shrink-0 text-muted-foreground">
          <ChevronDown className="size-4" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Positioner
          sideOffset={4}
          align="start"
          alignItemWithTrigger={false}
          className="z-50 outline-none"
        >
          <SelectPrimitive.Popup className="max-h-(--available-height) min-w-(--anchor-width) overflow-y-auto border bg-popover py-1 text-popover-foreground shadow-lg outline-none data-ending-style:opacity-0 data-starting-style:opacity-0 transition-opacity duration-100">
            <SelectPrimitive.List>
              {options.map((o) => (
                <SelectPrimitive.Item
                  key={o.value}
                  value={o.value}
                  disabled={o.disabled}
                  className="flex cursor-default items-center gap-2 py-1.5 pr-3 pl-2 text-sm outline-none select-none data-disabled:opacity-50 data-highlighted:bg-accent"
                >
                  <span className="flex size-4 shrink-0 items-center justify-center">
                    <SelectPrimitive.ItemIndicator>
                      <Check className="size-3.5" />
                    </SelectPrimitive.ItemIndicator>
                  </span>
                  {lead(o)}
                  <SelectPrimitive.ItemText>{o.label}</SelectPrimitive.ItemText>
                </SelectPrimitive.Item>
              ))}
            </SelectPrimitive.List>
          </SelectPrimitive.Popup>
        </SelectPrimitive.Positioner>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}
