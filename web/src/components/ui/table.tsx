import type * as React from "react";
import { cn } from "@/lib/utils";

// Plain table parts with the Adx spacing. Numeric columns take `numeric` so
// figures line up on the right in mono.

export function Table({ className, ...props }: React.ComponentProps<"table">) {
  return (
    <div className="w-full overflow-x-auto">
      <table className={cn("w-full border-collapse text-sm", className)} {...props} />
    </div>
  );
}

export function THead(props: React.ComponentProps<"thead">) {
  return <thead {...props} />;
}

export function TBody(props: React.ComponentProps<"tbody">) {
  return <tbody {...props} />;
}

export function TR({ className, ...props }: React.ComponentProps<"tr">) {
  return <tr className={cn("border-b last:border-b-0", className)} {...props} />;
}

export function TH({
  className,
  numeric,
  ...props
}: React.ComponentProps<"th"> & { numeric?: boolean }) {
  return (
    <th
      className={cn(
        "h-9 px-3 text-left font-mono text-[0.65rem] font-medium tracking-widest whitespace-nowrap text-muted-foreground uppercase first:pl-5 last:pr-5",
        numeric && "text-right",
        className,
      )}
      {...props}
    />
  );
}

export function TD({
  className,
  numeric,
  ...props
}: React.ComponentProps<"td"> & { numeric?: boolean }) {
  return (
    <td
      className={cn(
        "h-11 px-3 align-middle first:pl-5 last:pr-5",
        numeric && "text-right font-mono tabular-nums whitespace-nowrap",
        className,
      )}
      {...props}
    />
  );
}
