import { Link } from "@tanstack/react-router";
import {
  Activity,
  Bell,
  Bot,
  Cable,
  ChartColumn,
  ClipboardCheck,
  Eye,
  FileCheck,
  FileText,
  Filter,
  FolderOpen,
  Gauge,
  GraduationCap,
  Image,
  Inbox,
  Kanban,
  Landmark,
  ListChecks,
  type LucideIcon,
  MessageSquare,
  Palette,
  PanelLeftClose,
  PanelLeftOpen,
  Phone,
  Rocket,
  Search,
  Send,
  Shield,
  Target,
  Users,
  Workflow,
} from "lucide-react";
import { useState } from "react";
import { AccountMenu } from "@/components/account-menu";
import { ServerStatus } from "@/components/server-status";
import { WorkspaceSwitcher } from "@/components/workspace-switcher";
import { readStored, writeStored } from "@/lib/storage";
import { cn } from "@/lib/utils";

// The desk (design language, 09): a 200px sidebar on its own tone with the
// pages grouped by what they are for. Collapsed it is 48px and shows one
// 16px icon per page; the group headings become hairlines.
const NAV: { heading: string; items: { to: string; label: string; icon: LucideIcon }[] }[] = [
  {
    heading: "Work",
    items: [
      { to: "/projects", label: "Projects", icon: FolderOpen },
      { to: "/analytics", label: "Analytics", icon: ChartColumn },
      { to: "/ask", label: "Ask ADX", icon: MessageSquare },
    ],
  },
  {
    heading: "Create",
    items: [
      { to: "/brands", label: "Brands", icon: Palette },
      { to: "/research", label: "Research", icon: Search },
      { to: "/plans", label: "Plans", icon: FileText },
      { to: "/pages", label: "Pages", icon: Landmark },
      { to: "/creatives", label: "Creatives", icon: Image },
    ],
  },
  {
    heading: "Perform",
    items: [
      { to: "/launch", label: "Launch", icon: Rocket },
      { to: "/proposals", label: "Proposals", icon: FileCheck },
      { to: "/reports", label: "Reports", icon: ClipboardCheck },
      { to: "/alerts", label: "Alerts", icon: Bell },
    ],
  },
  {
    heading: "Convert",
    items: [
      { to: "/people", label: "People", icon: Users },
      { to: "/pipeline", label: "Pipeline", icon: Kanban },
      { to: "/calls", label: "Calls", icon: Phone },
      { to: "/leads", label: "Leads", icon: Inbox },
    ],
  },
  {
    heading: "Agents",
    items: [
      { to: "/supervisor", label: "Supervisor", icon: Eye },
      { to: "/workflows", label: "Workflows", icon: Workflow },
      { to: "/settings/agents", label: "Agents", icon: Bot },
      { to: "/learning", label: "Learning", icon: GraduationCap },
      { to: "/evaluation", label: "Evaluation", icon: Gauge },
    ],
  },
  {
    heading: "Workspace",
    items: [
      { to: "/connections", label: "Connections", icon: Cable },
      { to: "/settings/policies", label: "Policies", icon: Shield },
      { to: "/settings/funnel", label: "Funnel", icon: Filter },
      { to: "/settings/outreach", label: "Outreach", icon: Send },
      { to: "/settings/scoring", label: "Scoring", icon: Target },
      { to: "/settings/workspaces", label: "Workspaces", icon: Landmark },
      { to: "/ops", label: "Ops", icon: Activity },
      { to: "/runs", label: "Runs", icon: ListChecks },
    ],
  },
];

const WIDTH_KEY = "adx.sidebar";

export function Sidebar() {
  const [collapsed, setCollapsed] = useState(() => readStored(WIDTH_KEY) === "collapsed");
  const Toggle = collapsed ? PanelLeftOpen : PanelLeftClose;

  function toggle() {
    const next = !collapsed;
    setCollapsed(next);
    writeStored(WIDTH_KEY, next ? "collapsed" : "open");
  }

  return (
    <aside
      data-collapsed={collapsed || undefined}
      className={cn(
        "sticky top-0 flex h-screen shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground transition-[width] duration-300 ease-[cubic-bezier(0.2,0.7,0.2,1)] motion-reduce:transition-none",
        collapsed ? "w-12" : "w-[200px]",
      )}
    >
      <div
        className={cn(
          "flex items-center pt-5 pb-4",
          collapsed ? "flex-col gap-3" : "justify-between pr-2 pl-4",
        )}
      >
        <Link
          to="/projects"
          className="font-heading text-xl leading-none font-bold tracking-tight"
          aria-label="ADX home"
        >
          adx
        </Link>
        <button
          type="button"
          onClick={toggle}
          aria-label={collapsed ? "Open the sidebar" : "Collapse the sidebar"}
          aria-expanded={!collapsed}
          title={collapsed ? "Open the sidebar" : "Collapse the sidebar"}
          className="flex size-7 items-center justify-center text-muted-foreground hover:bg-sidebar-accent hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <Toggle size={16} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>
      <nav
        aria-label="Pages"
        className={cn(
          "flex-1 overflow-y-auto pb-4 [scrollbar-width:none]",
          collapsed ? "px-2" : "px-2",
        )}
      >
        {NAV.map((group, index) => (
          <div
            key={group.heading}
            className={cn(
              collapsed ? "flex flex-col items-center gap-1 py-2" : "mb-4",
              collapsed && index > 0 && "border-t",
            )}
          >
            {!collapsed && <p className="eyebrow px-2 pb-1.5">{group.heading}</p>}
            {group.items.map((item) => {
              const Icon = item.icon;
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  title={collapsed ? item.label : undefined}
                  aria-label={collapsed ? item.label : undefined}
                  className={cn(
                    "flex items-center hover:bg-sidebar-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                    collapsed ? "size-7 justify-center" : "h-8 gap-2 px-2",
                  )}
                  activeProps={{ className: "bg-sidebar-accent font-medium" }}
                >
                  <Icon size={16} strokeWidth={2} aria-hidden="true" className="shrink-0" />
                  {!collapsed && <span className="truncate">{item.label}</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>
      {!collapsed && (
        <div className="space-y-2 border-t px-4 py-3 text-xs">
          <WorkspaceSwitcher />
          <AccountMenu />
          <ServerStatus />
        </div>
      )}
    </aside>
  );
}
