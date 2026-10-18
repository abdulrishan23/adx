import { useNavigate } from "@tanstack/react-router";
import { LogOut, Monitor, Moon, Sun } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useTheme } from "@/components/theme-provider";
import type { ThemePreference } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { initials, useSession } from "./session";

// sidebar and rail open beside the sidebar, so the menu never covers the
// navigation; the phone drawer has no room beside it, so it opens upward.
type Placement = "sidebar" | "rail" | "drawer" | "topbar";

// AccountMenu is who's signed in, their display preference, and the way out.
// Sign out goes to the sign-in page until sign-in exists; then it ends the
// ADX session first.
export function AccountMenu({ placement }: { placement: Placement }) {
  const session = useSession();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const menuId = useId();
  const compact = placement === "rail" || placement === "topbar";
  const wide = placement === "sidebar" || placement === "drawer";

  useEffect(() => {
    if (!open) return;
    function onPointer(e: PointerEvent) {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onPointer);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div ref={root} className={cn("relative", wide && "w-full")}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={menuId}
        aria-label={compact ? `Account: ${session.name}` : undefined}
        title={compact ? session.name : undefined}
        onClick={() => setOpen((o) => !o)}
        className={cn(
          "flex items-center gap-2 rounded-md text-left transition-colors",
          "p-1",
          placement === "topbar" ? "hover:bg-accent" : "hover:bg-sidebar-accent",
          open && (placement === "topbar" ? "bg-accent" : "bg-sidebar-accent"),
          wide && "w-full",
        )}
      >
        <Avatar name={session.name} small={placement !== "topbar"} />
        {!compact && <span className="min-w-0 truncate text-sm font-medium">{session.name}</span>}
      </button>
      {open && (
        <div
          id={menuId}
          role="menu"
          className={cn(
            "absolute z-50 w-64 overflow-hidden rounded-lg border bg-popover text-popover-foreground shadow-lg",
            (placement === "sidebar" || placement === "rail") && "bottom-0 left-full ml-4",
            placement === "drawer" && "inset-x-0 bottom-full mb-2 w-auto",
            placement === "topbar" && "top-full right-0 mt-2",
          )}
        >
          <div className="border-b px-3 py-2.5">
            <p className="truncate text-sm font-medium">{session.name}</p>
            <p className="truncate text-xs text-muted-foreground">{session.email}</p>
          </div>
          <ThemeSwitch />
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setOpen(false);
              navigate({ to: "/sign-in" });
            }}
            className="flex w-full items-center gap-2 border-t px-3 py-2 text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
          >
            <LogOut className="size-4" />
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}

const THEMES: { value: ThemePreference; label: string; icon: typeof Sun }[] = [
  { value: "system", label: "System", icon: Monitor },
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
];

function ThemeSwitch() {
  const { preference, setPreference } = useTheme();
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2">
      <span className="text-sm" aria-hidden="true">
        Theme
      </span>
      <fieldset className="m-0 flex min-w-0 rounded-md border-0 bg-muted p-0.5">
        <legend className="sr-only">Theme</legend>
        {THEMES.map((t) => (
          <button
            key={t.value}
            type="button"
            aria-pressed={preference === t.value}
            aria-label={t.label}
            title={t.label}
            onClick={() => setPreference(t.value)}
            className={cn(
              "flex size-7 items-center justify-center rounded-none text-muted-foreground transition-colors hover:text-foreground",
              preference === t.value && "bg-card text-foreground shadow-xs",
            )}
          >
            <t.icon className="size-3.5" />
          </button>
        ))}
      </fieldset>
    </div>
  );
}

function Avatar({ name, small }: { name: string; small?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex shrink-0 items-center justify-center rounded-none bg-primary font-mono font-medium text-primary-foreground",
        small ? "size-6 text-[0.6rem]" : "size-8 text-xs",
      )}
    >
      {initials(name)}
    </span>
  );
}
