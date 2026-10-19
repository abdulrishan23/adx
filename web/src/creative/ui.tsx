import { type ReactNode, useRef, useState } from "react";
import type { ReviewStatus } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { cn } from "@/lib/utils";

export const REVIEW_LABEL: Record<ReviewStatus, string> = {
  approved: "Approved",
  revise: "Needs changes",
  waiting: "Waiting",
  unpublished: "Layout only",
};

const REVIEW_TONE: Record<ReviewStatus, Status | null> = {
  approved: "ready",
  revise: "caution",
  waiting: "pending",
  unpublished: null,
};

// A review state as a mark and words. The words stay ink; only the mark has
// colour.
export function ReviewState({ status, className }: { status: ReviewStatus; className?: string }) {
  const tone = REVIEW_TONE[status];
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-xs", className)}>
      {tone ? (
        <StatusMark status={tone} />
      ) : (
        <span
          aria-hidden="true"
          className="inline-block size-2 border border-dashed border-current"
        />
      )}
      {REVIEW_LABEL[status]}
    </span>
  );
}

// Counts of each review state, for a project row or a board header.
export function ReviewTally({
  approved,
  revise,
  waiting,
  className,
}: {
  approved: number;
  revise: number;
  waiting: number;
  className?: string;
}) {
  return (
    <span className={cn("inline-flex flex-wrap items-center gap-x-3 gap-y-1 text-xs", className)}>
      {(
        [
          ["approved", approved],
          ["revise", revise],
          ["waiting", waiting],
        ] as const
      ).map(([s, n]) => (
        <span key={s} className="inline-flex items-center gap-1.5">
          <StatusMark status={REVIEW_TONE[s] ?? "pending"} />
          <span className="font-mono tabular-nums">{n}</span>
          <span className="text-muted-foreground">{REVIEW_LABEL[s].toLowerCase()}</span>
        </span>
      ))}
    </span>
  );
}

// Dropzone takes files by drag and drop or a file picker.
export function Dropzone({
  title,
  hint,
  accept,
  multiple = true,
  disabled,
  onFiles,
  children,
}: {
  title: string;
  hint: ReactNode;
  accept: string;
  multiple?: boolean;
  disabled?: boolean;
  onFiles: (files: File[]) => void;
  children?: ReactNode;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const take = (list: FileList | null) => {
    const files = Array.from(list ?? []);
    if (files.length && !disabled) onFiles(files);
  };
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: the button inside is the accessible way in; dropping is a shortcut
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        take(e.dataTransfer.files);
      }}
      className={cn(
        "flex flex-col items-center gap-3 border border-dashed px-6 py-10 text-center transition-colors",
        over ? "border-foreground bg-muted" : "border-border",
      )}
    >
      <p className="font-medium">{title}</p>
      <p className="max-w-lg text-sm text-muted-foreground">{hint}</p>
      <Button
        type="button"
        variant="outline"
        disabled={disabled}
        onClick={() => input.current?.click()}
      >
        Choose files
      </Button>
      <input
        ref={input}
        type="file"
        multiple={multiple}
        accept={accept}
        className="hidden"
        onChange={(e) => {
          take(e.target.files);
          e.target.value = "";
        }}
      />
      {children}
    </div>
  );
}

// Progress is a thin square bar for uploads and generation.
export function Progress({ value, label }: { value: number; label: string }) {
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuenow={Math.round(value)}
      aria-valuemin={0}
      aria-valuemax={100}
      className="h-1.5 w-full max-w-sm bg-muted"
    >
      <div className="h-full bg-foreground transition-[width]" style={{ width: `${value}%` }} />
    </div>
  );
}

// Field is a label, its control and an optional hint.
export function Field({
  label,
  htmlFor,
  hint,
  children,
  className,
}: {
  label: string;
  htmlFor?: string;
  hint?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("space-y-1.5", className)}>
      <label htmlFor={htmlFor} className="text-sm font-medium">
        {label}
      </label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

export const textareaClass =
  "w-full min-w-0 border border-input bg-transparent px-2.5 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50";
