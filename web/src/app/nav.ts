import type { RegisteredRouter } from "@tanstack/react-router";
import {
  Activity,
  BookOpenText,
  Bot,
  ChartNoAxesCombined,
  ChartPie,
  Clapperboard,
  ClipboardCheck,
  Cog,
  Coins,
  Crosshair,
  FileText,
  Filter,
  FolderKanban,
  Gauge,
  Images,
  Inbox,
  Inbox as InboxIcon,
  Lightbulb,
  ListChecks,
  type LucideIcon,
  Megaphone,
  MessageSquareText,
  Palette,
  PhoneCall,
  Plug,
  RefreshCw,
  ShieldAlert,
  UserCog,
  UserSearch,
  Users,
  UsersRound,
} from "lucide-react";
import type { Role } from "@/api/types.legacy";
import { type Phase, released } from "@/lib/release";

export type RoutePath = keyof RegisteredRouter["routesByPath"];

export type NavItem = {
  label: string;
  to: RoutePath;
  icon: LucideIcon;
  // "soon" items have a route that shows what the screen will do. A feature
  // flips its item to "live" in the same PR that builds its screen.
  status: "live" | "soon";
  // One line on what the screen does, shown on its coming-soon page. Taken
  // from the features table in docs/reference.md.
  summary: string;
  // The build phase that ships this screen (docs/screens.md). Builds hide
  // anything past the release phase.
  phase: Phase;
  // Leave out to show the item to every role.
  roles?: readonly Role[];
};

// A door is a sub-product: Create, Perform or Convert. It opens at its first
// live screen, and its items show in the sidebar only while the door is open.
// A plain group lists its items all the time.
export type NavGroup = {
  label: string;
  items: readonly NavItem[];
  door?: { to: RoutePath; phase: Phase; icon: LucideIcon };
  // under lists path prefixes the group owns beyond its items, such as the
  // Projects screens reached from a campaign (plans, pages, launch).
  under?: readonly string[];
};

export const HOME: NavItem = {
  label: "Home",
  to: "/",
  icon: MessageSquareText,
  phase: 1,
  status: "live",
  summary: "Ask Adx, framed by where Create, Perform and Convert stand and what needs a person.",
};

export const INBOX: NavItem = {
  label: "Inbox",
  to: "/inbox",
  icon: InboxIcon,
  phase: 1,
  status: "live",
  summary: "Every card Adx has put in front of you: what needs you, and what Adx finished.",
};

// A project section is one of the five screens every project has, in the
// order the work happens (docs/screens.md). The sidebar shows them under the
// Projects door while a project is in the URL.
export type ProjectSection = {
  segment: "brand" | "media" | "studio" | "measurement" | "campaigns";
  label: string;
  icon: LucideIcon;
  phase: Phase;
  summary: string;
};

export const PROJECT_SECTIONS: readonly ProjectSection[] = [
  {
    segment: "brand",
    label: "Brand, facts and research",
    icon: BookOpenText,
    phase: 1,
    summary:
      "The brand kit, the facts Adx may state about this project, its offers, and the research behind them.",
  },
  {
    segment: "media",
    label: "Media",
    icon: ChartPie,
    phase: 2,
    summary: "The project's media plans and their versions.",
  },
  {
    segment: "studio",
    label: "Studio",
    icon: Palette,
    phase: 1,
    summary: "The project's creatives and landing pages.",
  },
  {
    segment: "measurement",
    label: "Measurement",
    icon: Crosshair,
    phase: 1,
    summary:
      "What's measured for this project and whether it works: conversion actions, pixels and outcome sync. Figures stay under Perform.",
  },
  {
    segment: "campaigns",
    label: "Campaigns",
    icon: Megaphone,
    phase: 2,
    summary: "The project's pushes, each from brief to live.",
  },
];

export function projectSectionPath(segment: ProjectSection["segment"]): RoutePath {
  return `/projects/$projectId/${segment}` as RoutePath;
}

// projectIdFrom reads the project a path is scoped to, or null outside one.
// The Projects list and New project carry no project.
export function projectIdFrom(path: string): number | null {
  const m = /^\/projects\/(\d+)(?:\/|$)/.exec(path);
  return m ? Number(m[1]) : null;
}

export const NAV: readonly NavGroup[] = [
  {
    label: "Projects",
    door: { to: "/projects", phase: 1, icon: FolderKanban },
    under: ["/studio/", "/brands/", "/competitors"],
    items: [
      {
        label: "Projects",
        to: "/projects",
        icon: FolderKanban,
        phase: 1,
        status: "live",
        summary:
          "Each development Adx markets, such as The Wadi, and its campaigns: the brief, the ads Adx writes, the page, the plan and the launch for each push.",
      },
    ],
  },
  {
    label: "Creative",
    door: { to: "/creative/concepts", phase: 1, icon: Palette },
    // A concept's page, the ad editor and uploads live under a project's path.
    under: ["/creative/"],
    items: [
      {
        label: "Concepts",
        to: "/creative/concepts",
        icon: Lightbulb,
        phase: 1,
        status: "live",
        summary: "Every idea across projects and campaigns, and the ads built from it.",
      },
      {
        label: "Library",
        to: "/creative/library",
        icon: Images,
        phase: 1,
        status: "live",
        summary:
          "Each project's pictures, logos, fonts and colours, for every campaign to draw on.",
      },
      {
        label: "Producer",
        to: "/creative/producer",
        icon: Clapperboard,
        phase: 1,
        status: "live",
        summary:
          "Every ad across campaigns: what needs changes, what's awaiting review, what's approved.",
      },
    ],
  },
  {
    label: "Perform",
    door: { to: "/analytics", phase: 1, icon: Activity },
    under: ["/data-quality", "/outcomes", "/website", "/organic"],
    items: [
      {
        label: "Analytics",
        to: "/analytics",
        icon: ChartNoAxesCombined,
        phase: 1,
        status: "live",
        summary:
          "Spend, lead quality and trends by project and ad source, with period comparisons and current alerts.",
      },
      {
        label: "Campaigns",
        to: "/campaigns",
        icon: Megaphone,
        phase: 1,
        status: "live",
        summary:
          "Campaign, ad set and ad views with spend, leads, qualified leads, deals and the temperature mix.",
      },
      {
        label: "Revenue",
        to: "/revenue",
        icon: Coins,
        phase: 1,
        status: "live",
        summary: "Spend against Done-Deal revenue by month and by channel, and where leads live.",
      },
      {
        label: "Reports",
        to: "/reports",
        icon: FileText,
        phase: 1,
        status: "live",
        summary: "Monthly and weekly reports for the CMO and CEO.",
      },
      {
        label: "Audiences",
        to: "/audiences",
        icon: UsersRound,
        phase: 3,
        status: "live",
        summary: "Meta custom audiences and Google Customer Match built from leads that went cold.",
      },
    ],
  },
  {
    label: "Convert",
    door: { to: "/leads", phase: 1, icon: Filter },
    items: [
      {
        label: "Leads",
        to: "/leads",
        icon: Users,
        phase: 1,
        status: "live",
        summary: "Every lead with its source, fit, intent and temperature, and why.",
      },
      {
        label: "Enrichment",
        to: "/enrichment",
        icon: UserSearch,
        phase: 4,
        status: "live",
        summary:
          "Look leads up beyond the form and get a brief for each, with the queue of lookups and what's left this month.",
      },
      {
        label: "Calls",
        to: "/convert/calls",
        icon: PhoneCall,
        phase: 4,
        status: "soon",
        summary:
          "Calls the voice agent made and the ones it handed to a person, with transcripts and outcomes.",
      },
      {
        label: "Inbox",
        to: "/convert/inbox",
        icon: Inbox,
        phase: 5,
        status: "soon",
        summary:
          "WhatsApp, email and SMS in one place, with Adx's note on each lead and a drafted reply.",
      },
      {
        label: "CRM sync",
        to: "/convert/crm",
        icon: RefreshCw,
        phase: 5,
        status: "soon",
        summary: "Stages, owners and notes kept in step with GoHighLevel, and what failed to sync.",
      },
    ],
  },
  {
    label: "Workspace",
    items: [
      {
        label: "Agents",
        to: "/workflows",
        icon: Bot,
        phase: 5,
        status: "soon",
        summary:
          "The agents that run on a schedule, what each one did last, and the ones waiting on a person.",
      },
      {
        label: "Engine",
        to: "/engine",
        icon: Cog,
        phase: 1,
        status: "live",
        summary:
          "The server's own screens, on real data, for every capability our screens haven't been wired to yet.",
      },
    ],
  },
  {
    label: "Settings",
    items: [
      {
        label: "Connections",
        to: "/settings/connections",
        icon: Plug,
        phase: 0,
        status: "live",
        summary:
          "Connect Google Ads, Meta, GoHighLevel and the other sources, and see each one's health and last sync.",
        roles: ["admin"],
      },
      {
        label: "People",
        to: "/settings/people",
        icon: UserCog,
        phase: 0,
        status: "live",
        summary:
          "Invite people, set their roles and choose which write actions marketers may take.",
        roles: ["admin"],
      },
      {
        label: "Stages and lists",
        to: "/settings/lists",
        icon: ListChecks,
        phase: 1,
        status: "live",
        summary: "The status-to-stage table, project aliases and the page catalogue.",
        roles: ["admin"],
      },
      {
        label: "Guardrails",
        to: "/settings/guardrails",
        icon: ShieldAlert,
        phase: 0,
        status: "live",
        summary:
          "Caps on spend increases and changes per day, monthly budgets, and a kill switch per account.",
        roles: ["admin"],
      },
      {
        label: "Change log",
        to: "/proposals",
        icon: ClipboardCheck,
        phase: 3,
        status: "live",
        roles: ["admin", "marketer"],
        summary:
          "Every change Adx made to Google, Meta and GoHighLevel, and the ones still waiting for approval. Approvals themselves arrive in the Inbox.",
      },
      {
        label: "Scoring",
        to: "/settings/scoring",
        icon: Gauge,
        phase: 4,
        status: "live",
        summary:
          "Fit and intent weights, temperature thresholds, and how well each temperature predicted sales.",
        roles: ["admin"],
      },
    ],
  },
];

// navFor returns the groups a role can see in this release, dropping groups
// left empty.
export function navFor(role: Role): NavGroup[] {
  return NAV.map((group) => ({
    ...group,
    items: group.items.filter(
      (item) => released(item.phase) && (!item.roles || item.roles.includes(role)),
    ),
  })).filter((group) => group.items.length > 0 && (!group.door || released(group.door.phase)));
}

export function findNavItem(path: string): NavItem | undefined {
  if (path === HOME.to) {
    return HOME;
  }
  if (path === INBOX.to) {
    return INBOX;
  }
  for (const group of NAV) {
    const item = group.items.find((i) => i.to === path);
    if (item) {
      return item;
    }
  }
  return undefined;
}

// doorOf says which door a path belongs to, so the sidebar knows which one to
// open. A screen outside every door, such as Settings, returns undefined.
export function doorOf(path: string): NavGroup | undefined {
  return NAV.find(
    (g) =>
      g.door &&
      (g.items.some((i) => path === i.to || path.startsWith(`${i.to}/`)) ||
        (g.under ?? []).some((u) => path.startsWith(u))),
  );
}
