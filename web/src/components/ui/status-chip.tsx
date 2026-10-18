import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";
import { cn } from "@/lib/utils";

// Status chips are the only colour in the Adx look. Ready, caution and error
// are solid and vivid; pending is an outline, for anything waiting or loading.
const statusChipVariants = cva(
  "inline-flex h-6 shrink-0 items-center rounded-sm px-2.5 font-mono text-[0.7rem] font-semibold tracking-wider whitespace-nowrap uppercase",
  {
    variants: {
      status: {
        ready: "bg-status-ready text-status-ready-foreground",
        caution: "bg-status-caution text-status-caution-foreground",
        error: "bg-status-error text-status-error-foreground",
        pending: "border-[1.5px] border-foreground px-2 text-foreground",
      },
    },
    defaultVariants: { status: "pending" },
  },
);

export type Status = NonNullable<VariantProps<typeof statusChipVariants>["status"]>;

function StatusChip({
  className,
  status,
  children,
  ...props
}: React.ComponentProps<"span"> & VariantProps<typeof statusChipVariants>) {
  return (
    <span
      data-slot="status-chip"
      className={cn(statusChipVariants({ status }), className)}
      {...props}
    >
      {children}
    </span>
  );
}

export { StatusChip, statusChipVariants };

// StatusMark is the chip's colour as a small square, for a line of text or a
// tab that needs a status. The text beside it stays ink, so it stays readable.
const MARK: Record<Status, string> = {
  ready: "bg-status-ready",
  caution: "bg-status-caution",
  error: "bg-status-error",
  pending: "border-[1.5px] border-current",
};

export function StatusMark({ status, className }: { status: Status; className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn("inline-block size-2 shrink-0", MARK[status], className)}
    />
  );
}
