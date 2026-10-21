import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { Menu, PanelLeftClose, PanelLeftOpen, Search, X } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { needsYou, useHomeDesk } from "@/api/fronts";
import { AdxMark, AdxWordmark } from "@/components/brand";
import { CommandPalette, useCommandPaletteHotkey } from "@/components/command-palette";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { AccountMenu } from "./account-menu";
import { doorOf, HOME, INBOX, type NavGroup, type NavItem, navFor } from "./nav";
import { ScopeProvider } from "./scope";
import { useSession } from "./session";

const COLLAPSED_KEY = "adx-sidebar-collapsed";

function readCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

// AppShell is the signed-in layout: a sidebar on desktop, a top bar with a
// drawer on small screens, and the page in the middle.
export function AppShell() {
  const session = useSession();
  const groups = navFor(session.role);
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const openSearch = useCallback(() => setSearchOpen(true), []);
  useCommandPaletteHotkey(openSearch);
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  // Close the drawer whenever the page changes.
  // biome-ignore lint/correctness/useExhaustiveDependencies: runs on navigation only
  useEffect(() => setDrawerOpen(false), [pathname]);

  function toggleCollapsed() {
    setCollapsed((c) => {
      try {
        localStorage.setItem(COLLAPSED_KEY, c ? "0" : "1");
      } catch {
        // Storage can be blocked; the sidebar still toggles for this visit.
      }
      return !c;
    });
  }

  return (
    <ScopeProvider>
      <div className="flex min-h-screen">
        <aside
          className={cn(
            "sticky top-0 z-30 hidden h-screen shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground transition-[width] duration-200 md:flex",
            collapsed ? "w-12" : "w-[12.5rem]",
          )}
        >
          <div className={cn("flex h-14 items-center", collapsed ? "justify-center" : "pr-1 pl-3")}>
            {collapsed ? (
              <button
                type="button"
                aria-label="Expand sidebar"
                title="Expand sidebar"
                onClick={toggleCollapsed}
                className="group flex size-9 items-center justify-center rounded-md hover:bg-sidebar-accent"
              >
                <AdxMark className="group-hover:hidden group-focus-visible:hidden" />
                <PanelLeftOpen className="hidden size-4 group-hover:block group-focus-visible:block" />
              </button>
            ) : (
              <>
                <Link to="/" aria-label="Adx home">
                  <AdxWordmark />
                </Link>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="ml-auto text-muted-foreground"
                  aria-label="Collapse sidebar"
                  title="Collapse sidebar"
                  onClick={toggleCollapsed}
                >
                  <PanelLeftClose />
                </Button>
              </>
            )}
          </div>
          <SearchButton collapsed={collapsed} onClick={openSearch} />
          <SidebarNav groups={groups} collapsed={collapsed} pathname={pathname} />
          <SidebarFooter collapsed={collapsed} />
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b bg-background/90 px-4 backdrop-blur md:hidden">
            <Button
              variant="ghost"
              size="icon"
              aria-label="Open menu"
              onClick={() => setDrawerOpen(true)}
            >
              <Menu />
            </Button>
            <Link to="/" aria-label="Adx home">
              <AdxWordmark />
            </Link>
            <span className="ml-auto flex items-center gap-1">
              <Button variant="ghost" size="icon" aria-label="Search" onClick={openSearch}>
                <Search />
              </Button>
              <AccountMenu placement="topbar" />
            </span>
          </header>

          <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 md:px-8 md:py-10">
            <Outlet />
          </main>
        </div>

        <CommandPalette open={searchOpen} onOpenChange={setSearchOpen} />

        {drawerOpen && (
          <div className="fixed inset-0 z-40 md:hidden">
            <button
              type="button"
              aria-label="Close menu"
              className="absolute inset-0 bg-foreground/30"
              onClick={() => setDrawerOpen(false)}
            />
            <aside className="absolute inset-y-0 left-0 flex w-72 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground shadow-xl">
              <div className="flex h-14 items-center justify-between pr-2 pl-4">
                <AdxWordmark />
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Close menu"
                  onClick={() => setDrawerOpen(false)}
                >
                  <X />
                </Button>
              </div>
              <SidebarNav groups={groups} collapsed={false} pathname={pathname} />
              <SidebarFooter collapsed={false} inDrawer />
            </aside>
          </div>
        )}
      </div>
    </ScopeProvider>
  );
}

function SidebarNav({
  groups,
  collapsed,
  pathname,
}: {
  groups: NavGroup[];
  collapsed: boolean;
  pathname: string;
}) {
  const open = doorOf(pathname);
  const desk = useHomeDesk();
  const inboxCount = desk.data?.cards.filter(needsYou).length ?? 0;
  const doors = groups.filter((g) => g.door);
  const rest = groups.filter((g) => !g.door);
  return (
    <nav className="no-scrollbar flex-1 space-y-4 overflow-y-auto px-1.5 py-2" aria-label="Main">
      <ul className="space-y-0.5">
        <li>
          <NavLink item={HOME} collapsed={collapsed} exact />
        </li>
        <li>
          <NavLink item={INBOX} collapsed={collapsed} badge={inboxCount} />
        </li>
        {doors.map((g) => (
          <li key={g.label}>
            {g.door && g.items[0] && (
              <NavLink
                item={{ ...g.items[0], label: g.label, to: g.door.to, icon: g.door.icon }}
                collapsed={collapsed}
                active={open?.label === g.label}
              />
            )}
            {!collapsed && open?.label === g.label && g.items.length > 1 && (
              <ul className="my-1 ml-3 space-y-0.5 border-l pl-1.5">
                {g.items.map((item) => (
                  <li key={item.to}>
                    <NavLink item={item} collapsed={false} sub />
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
      {rest.map((group) => (
        <div key={group.label}>
          {collapsed ? (
            <div className="mx-2 mb-2 border-t border-sidebar-border" />
          ) : (
            <p className="px-2 pb-1 font-mono text-[0.6rem] tracking-widest text-muted-foreground uppercase">
              {group.label}
            </p>
          )}
          <ul className="space-y-0.5">
            {group.items.map((item) => (
              <li key={item.to}>
                <NavLink item={item} collapsed={collapsed} />
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}

function NavLink({
  item,
  collapsed,
  sub,
  exact,
  active,
  badge,
}: {
  item: NavItem;
  collapsed: boolean;
  sub?: boolean;
  exact?: boolean;
  // A count of things waiting, shown at the right; zero shows nothing.
  badge?: number;
  // Set for a door, which is "open" while any screen under it is showing.
  active?: boolean;
}) {
  return (
    <Link
      to={item.to}
      title={collapsed ? item.label : undefined}
      activeOptions={{ exact }}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex items-center gap-2 px-2 text-sidebar-foreground/75 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground",
        sub ? "h-7 text-[0.8rem]" : "h-8 text-sm",
        collapsed && "justify-center px-0",
        active && "bg-sidebar-accent font-medium text-sidebar-foreground",
      )}
      activeProps={{
        className: "bg-sidebar-accent font-medium text-sidebar-foreground",
      }}
    >
      {!sub && <item.icon className="size-4 shrink-0" />}
      {!collapsed && (
        <>
          <span className="truncate">{item.label}</span>
          {!!badge && (
            <span className="ml-auto font-mono text-[0.65rem] tabular-nums text-muted-foreground">
              {badge}
            </span>
          )}
          {item.status === "soon" && (
            <span className="ml-auto font-mono text-[0.55rem] tracking-wider text-muted-foreground uppercase">
              Soon
            </span>
          )}
        </>
      )}
    </Link>
  );
}

function SidebarFooter({ collapsed, inDrawer }: { collapsed: boolean; inDrawer?: boolean }) {
  return (
    <div className={cn("border-t border-sidebar-border", collapsed && "flex justify-center p-1.5")}>
      <AccountMenu placement={inDrawer ? "drawer" : collapsed ? "rail" : "sidebar"} />
    </div>
  );
}

const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

function SearchButton({ collapsed, onClick }: { collapsed: boolean; onClick: () => void }) {
  if (collapsed) {
    return (
      <div className="flex justify-center pb-1">
        <Button variant="ghost" size="icon-sm" aria-label="Search" title="Search" onClick={onClick}>
          <Search />
        </Button>
      </div>
    );
  }
  return (
    <div className="px-2 pb-1">
      <button
        type="button"
        onClick={onClick}
        className="flex h-8 w-full items-center gap-2 rounded-md border border-sidebar-border bg-background/60 px-2.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <Search className="size-4" aria-hidden="true" />
        Search or ask
        {/* Mono fonts draw ⌘ smaller than the K, so the Mac hint uses the
            system face, which sizes both alike. */}
        <kbd
          className={cn(
            "ml-auto",
            IS_MAC ? "font-[system-ui] text-xs" : "font-mono text-[0.65rem]",
          )}
        >
          {IS_MAC ? "⌘K" : "Ctrl K"}
        </kbd>
      </button>
    </div>
  );
}
