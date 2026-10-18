import { Dialog } from "@base-ui/react/dialog";
import { useNavigate } from "@tanstack/react-router";
import { Megaphone, MessageSquareText, Search, UserRound } from "lucide-react";
import { type KeyboardEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { useCampaigns, useLeads } from "@/api/adx";
import { projectOf } from "@/api/mock/data";
import { navFor } from "@/app/nav";
import { useSession } from "@/app/session";
import { cn } from "@/lib/utils";

type Item = {
  id: string;
  group: "Ask" | "Screens" | "Leads" | "Campaigns";
  label: string;
  hint?: string;
  icon: ReactNode;
  run: () => void;
};

// Opens on ⌘K or Ctrl+K from anywhere in the app.
export function useCommandPaletteHotkey(onOpen: () => void) {
  useEffect(() => {
    function onKey(e: globalThis.KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        onOpen();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onOpen]);
}

const LIMIT = 5;

export function CommandPalette({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="fixed inset-0 z-40 bg-foreground/25 transition-opacity data-[ending-style]:opacity-0 data-[starting-style]:opacity-0" />
        <Dialog.Popup className="fixed top-[12vh] left-1/2 z-50 w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 overflow-hidden rounded-xl border bg-background shadow-2xl outline-none transition-[opacity,scale] data-[ending-style]:scale-95 data-[ending-style]:opacity-0 data-[starting-style]:scale-95 data-[starting-style]:opacity-0">
          <Dialog.Title className="sr-only">Search Adx</Dialog.Title>
          {open && <Palette onClose={() => onOpenChange(false)} />}
        </Dialog.Popup>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Palette({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate();
  const session = useSession();
  const leads = useLeads();
  const campaigns = useCampaigns();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const list = useRef<HTMLDivElement>(null);

  const items = useMemo<Item[]>(() => {
    const q = query.trim().toLowerCase();
    const has = (s: string) => s.toLowerCase().includes(q);
    const go = (fn: () => void) => () => {
      onClose();
      fn();
    };
    const out: Item[] = [];
    if (q) {
      out.push({
        id: "ask",
        group: "Ask",
        label: `Ask Adx: "${query.trim()}"`,
        icon: <MessageSquareText />,
        run: go(() => navigate({ to: "/ask", search: { q: query.trim() } })),
      });
    }
    for (const g of navFor(session.role)) {
      for (const n of g.items) {
        if (!q || has(n.label) || has(g.label)) {
          const Icon = n.icon;
          out.push({
            id: `nav-${n.to}`,
            group: "Screens",
            label: n.label,
            hint: g.label,
            icon: <Icon />,
            run: go(() => navigate({ to: n.to })),
          });
        }
      }
    }
    if (q) {
      for (const l of (leads.data ?? []).filter((l) => has(l.name)).slice(0, LIMIT)) {
        out.push({
          id: `lead-${l.id}`,
          group: "Leads",
          label: l.name,
          hint: `${l.projectId ? projectOf(l.projectId) : "No project"} · ${l.owner}`,
          icon: <UserRound />,
          run: go(() => navigate({ to: "/leads/$leadId", params: { leadId: String(l.id) } })),
        });
      }
      for (const c of (campaigns.data ?? []).filter((c) => has(c.name)).slice(0, LIMIT)) {
        out.push({
          id: `campaign-${c.id}`,
          group: "Campaigns",
          label: c.name,
          hint: c.platform === "google" ? "Google" : "Meta",
          icon: <Megaphone />,
          run: go(() => navigate({ to: "/campaigns", search: { open: c.id } })),
        });
      }
    }
    return out;
  }, [query, leads.data, campaigns.data, session.role, navigate, onClose]);

  // A new query starts at the top of the list.
  // biome-ignore lint/correctness/useExhaustiveDependencies: reset on query change only
  useEffect(() => setActive(0), [query]);

  useEffect(() => {
    list.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(items.length - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      items[active]?.run();
    }
  }

  const groups = [...new Set(items.map((i) => i.group))];
  const activeId = items[active] ? `cmd-${items[active].id}` : undefined;

  return (
    <div>
      <div className="flex items-center gap-2 border-b px-4">
        <Search className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <input
          // biome-ignore lint/a11y/noAutofocus: the palette exists to be typed into
          autoFocus
          role="combobox"
          aria-expanded="true"
          aria-controls="cmd-list"
          aria-activedescendant={activeId}
          aria-label="Search screens, leads and campaigns, or ask Adx"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="Search screens, leads and campaigns, or ask Adx"
          className="h-12 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
        <kbd className="border px-1.5 font-mono text-[0.65rem] text-muted-foreground">Esc</kbd>
      </div>
      <div ref={list} id="cmd-list" role="listbox" className="max-h-[50vh] overflow-y-auto p-2">
        {items.length === 0 && (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">Nothing matches.</p>
        )}
        {groups.map((g) => (
          // biome-ignore lint/a11y/useSemanticElements: a group inside a listbox, not a form fieldset
          <div key={g} role="group" aria-label={g} className="mb-1">
            {g !== "Ask" && (
              <p className="px-3 pt-2 pb-1 font-mono text-[0.65rem] tracking-widest text-muted-foreground uppercase">
                {g}
              </p>
            )}
            {items.map((item, i) =>
              item.group === g ? (
                <div
                  key={item.id}
                  id={`cmd-${item.id}`}
                  role="option"
                  tabIndex={-1}
                  aria-selected={i === active}
                  data-index={i}
                  onMouseMove={() => setActive(i)}
                  onClick={item.run}
                  onKeyDown={(e) => e.key === "Enter" && item.run()}
                  className={cn(
                    "flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-muted-foreground",
                    i === active && "bg-muted",
                  )}
                >
                  {item.icon}
                  <span className="min-w-0 flex-1 truncate">{item.label}</span>
                  {item.hint && (
                    <span className="shrink-0 text-xs text-muted-foreground">{item.hint}</span>
                  )}
                </div>
              ) : null,
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
