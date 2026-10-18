import { useId } from "react";
import { cn } from "@/lib/utils";

// AdxMark is the +x knocked out of a square (B3). It draws in currentColor, so
// it follows the theme. The mask id is per instance, since several marks can
// be on one page.
export function AdxMark({ className }: { className?: string }) {
  const maskId = useId();
  return (
    <svg viewBox="0 0 1000 1000" className={cn("size-6", className)} role="img" aria-label="Adx">
      <defs>
        <mask id={maskId} maskUnits="userSpaceOnUse" x="0" y="0" width="1000" height="1000">
          <rect width="1000" height="1000" fill="white" />
          <rect x="150" y="441.67" width="333.33" height="116.67" fill="black" />
          <rect x="258.33" y="333.33" width="116.67" height="333.33" fill="black" />
          <g transform="rotate(45 691.67 500)">
            <rect x="491.67" y="441.67" width="400" height="116.67" fill="black" />
            <rect x="633.33" y="300" width="116.67" height="400" fill="black" />
          </g>
        </mask>
      </defs>
      <rect width="1000" height="1000" fill="currentColor" mask={`url(#${maskId})`} />
    </svg>
  );
}

export function AdxWordmark({ className }: { className?: string }) {
  return (
    <span className={cn("flex items-center gap-2", className)}>
      <AdxMark className="size-6 shrink-0" />
      <span className="font-heading text-xl leading-none font-bold tracking-tight">adx</span>
    </span>
  );
}

// The client whose workspace this is. Drop the client's logo file into
// public/brand and set `logo` to show it; until then the name is shown as text.
export const CLIENT = {
  name: "Reportage",
  logo: null as string | null,
};

export function ClientMark({ className }: { className?: string }) {
  if (CLIENT.logo) {
    return <img src={CLIENT.logo} alt={CLIENT.name} className={cn("h-5 w-auto", className)} />;
  }
  return (
    <span className={cn("text-xs font-semibold tracking-[0.2em] uppercase", className)}>
      {CLIENT.name}
    </span>
  );
}
