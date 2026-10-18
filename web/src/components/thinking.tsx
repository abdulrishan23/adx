import { cn } from "@/lib/utils";

// ThinkingMark is the + from the Adx mark, turning into an x and back while
// Adx works. It holds still for people who ask for reduced motion.
export function ThinkingMark({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 12 12"
      aria-hidden="true"
      className={cn("size-3 shrink-0 animate-think motion-reduce:animate-none", className)}
    >
      <rect x="0" y="4" width="12" height="4" fill="currentColor" />
      <rect x="4" y="0" width="4" height="12" fill="currentColor" />
    </svg>
  );
}
