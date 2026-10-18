import { Link, useNavigate } from "@tanstack/react-router";
import {
  Activity,
  ArrowUpRight,
  BatteryLow,
  CalendarRange,
  ChevronDown,
  ChevronUp,
  CircleOff,
  Columns3,
  Eye,
  FileText,
  FileWarning,
  Filter,
  Gauge,
  History,
  Inbox,
  type LucideIcon,
  MessageCircleQuestion,
  PenTool,
  PhoneCall,
  Presentation,
  RefreshCcw,
  Rocket,
  ScrollText,
  SearchX,
  TrendingUp,
  Wallet,
  Waypoints,
} from "lucide-react";
import { useEffect, useState } from "react";
import { type Card, type Template, type TemplateIcon, useHomeDesk } from "@/api/fronts";
import { useSession } from "@/app/session";
import { AdxMark } from "@/components/brand";
import { CardView } from "@/components/front/card";
import { ErrorNote, LoadingRows } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Composer } from "./ask";
import { SampleChip } from "./create/shared";
import { inboxOrder } from "./inbox";

// Home is a desk: a greeting, the cards that deserve attention today (any
// door, any shape, each with a question ready to ask about it), the chat bar,
// and ready-to-fire prompts below.
export function HomePage() {
  const desk = useHomeDesk();
  const navigate = useNavigate();
  const ask = (q: string) => navigate({ to: "/ask", search: { q } });
  // Opening the template catalog folds the carousel away, so the bar rises
  // and the catalog has the page.
  const [catalog, setCatalog] = useState(false);

  return (
    <>
      {desk.error && <ErrorNote error={desk.error} />}
      {desk.isPending && <LoadingRows rows={8} />}
      {desk.data && (
        <div className="flex flex-col gap-8">
          <section>
            <div className="mb-5 flex items-start justify-between gap-4">
              <div className="min-w-0">
                <Greeting />
                <p className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-muted-foreground">
                  {desk.data.summary}
                  {desk.data.sample && <SampleChip />}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Link to="/inbox" className={buttonVariants({ variant: "ghost", size: "sm" })}>
                  <Inbox /> Inbox
                </Link>
                <Link to="/present" className={buttonVariants({ variant: "ghost", size: "sm" })}>
                  <Presentation /> Present
                </Link>
              </div>
            </div>
            <div
              aria-hidden={catalog}
              inert={catalog}
              className={cn(
                "grid transition-[grid-template-rows,opacity] duration-500 ease-[cubic-bezier(.2,.7,.2,1)]",
                catalog ? "grid-rows-[0fr] opacity-0" : "grid-rows-[1fr] opacity-100",
              )}
            >
              <div className="min-h-0 overflow-hidden">
                <Carousel cards={inboxOrder(desk.data.cards)} onAsk={ask} paused={catalog} />
              </div>
            </div>
          </section>

          <section className="mx-auto w-full max-w-3xl">
            <div className="mb-4 flex items-center justify-center gap-3">
              <AdxMark className="size-7" />
              <h1 className="font-heading text-2xl font-semibold tracking-tight">Ask Adx</h1>
            </div>
            <Composer
              onSend={ask}
              busy={false}
              size="large"
              placeholder="Ask about any brand, figure or lead, or tell Adx what to do"
            />
          </section>
          <Templates
            templates={desk.data.templates}
            onAsk={ask}
            open={catalog}
            onOpenChange={(next) => {
              setCatalog(next);
              window.scrollTo({ top: 0, behavior: "smooth" });
            }}
          />
        </div>
      )}
    </>
  );
}

const SLIDE_MS = 4000;

// Carousel shows the cards three at a time in a fixed-height row and slides
// one step every few seconds. Hovering holds it; arrows and dots move it.
function Carousel({
  cards,
  onAsk,
  paused = false,
}: {
  cards: Card[];
  onAsk: (q: string) => void;
  paused?: boolean;
}) {
  const [index, setIndex] = useState(0);
  const [held, setHeld] = useState(false);
  const [perView, setPerView] = useState(3);
  const n = cards.length;
  const steps = Math.max(1, n - perView + 1);

  useEffect(() => {
    const update = () =>
      setPerView(
        window.innerWidth < 640
          ? 1
          : window.innerWidth < 1024
            ? 2
            : window.innerWidth < 1400
              ? 3
              : 4,
      );
    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  useEffect(() => {
    if (held || paused || steps <= 1) return;
    const t = setInterval(() => setIndex((i) => (i + 1) % steps), SLIDE_MS);
    return () => clearInterval(t);
  }, [held, paused, steps]);

  const at = Math.min(index, steps - 1);
  const gap = 16;
  return (
    <section
      aria-roledescription="carousel"
      aria-label="Today"
      className="group/carousel relative"
      onMouseEnter={() => setHeld(true)}
      onMouseLeave={() => setHeld(false)}
      onFocus={() => setHeld(true)}
      onBlur={() => setHeld(false)}
    >
      <div className="overflow-hidden">
        <ul
          className="flex transition-transform duration-500 ease-[cubic-bezier(.2,.7,.2,1)]"
          style={{
            gap,
            transform: `translateX(calc(-${at} * ((100% - ${(perView - 1) * gap}px) / ${perView} + ${gap}px)))`,
          }}
        >
          {cards.map((c) => (
            <li
              key={c.id}
              className="shrink-0"
              style={{ width: `calc((100% - ${(perView - 1) * gap}px) / ${perView})` }}
            >
              <CardView card={c} delay={0} onAsk={onAsk} />
            </li>
          ))}
        </ul>
      </div>
      {/* The dots only show while the pointer or keyboard focus is on the
          carousel; the space stays, so nothing below moves. */}
      <div className="mt-4 flex items-center justify-center opacity-0 transition-opacity duration-200 group-focus-within/carousel:opacity-100 group-hover/carousel:opacity-100">
        <div className="flex gap-1.5">
          {Array.from({ length: steps }, (_, i) => (
            <button
              // biome-ignore lint/suspicious/noArrayIndexKey: dots are positional
              key={i}
              type="button"
              aria-label={`Slide ${i + 1}`}
              aria-current={i === at}
              onClick={() => setIndex(i)}
              className={cn(
                "h-[3px] w-6 transition-colors",
                i === at ? "bg-foreground" : "bg-border hover:bg-muted-foreground/50",
              )}
            />
          ))}
        </div>
      </div>
    </section>
  );
}

// After midnight is still the evening, not the morning.
const PART_OF_DAY = (h: number) =>
  h < 5 ? "Good evening" : h < 12 ? "Good morning" : h < 17 ? "Good afternoon" : "Good evening";

// Greeting says hello. What needs a person is on the cards themselves.
function Greeting() {
  const session = useSession();
  const hour = Number(
    new Intl.DateTimeFormat("en-GB", {
      hour: "numeric",
      hour12: false,
      timeZone: "Asia/Dubai",
    }).format(new Date()),
  );
  const first = session.name.split(" ")[0] ?? session.name;
  return (
    <h1 className="font-heading text-3xl font-semibold tracking-tight">
      {PART_OF_DAY(hour)}, {first}
    </h1>
  );
}

const TEMPLATE_GLYPH: Record<TemplateIcon, LucideIcon> = {
  changes: History,
  cost: TrendingUp,
  waste: CircleOff,
  call: PhoneCall,
  pace: Gauge,
  compare: Columns3,
  budget: Wallet,
  exclude: SearchX,
  revive: RefreshCcw,
  reasons: MessageCircleQuestion,
  sources: Waypoints,
  forms: FileWarning,
  launch: Rocket,
  fatigue: BatteryLow,
  review: Eye,
  plan: CalendarRange,
  report: FileText,
  summary: ScrollText,
};

function TemplateGlyph({ icon, className }: { icon: TemplateIcon; className?: string }) {
  const Icon = TEMPLATE_GLYPH[icon];
  return <Icon className={className} aria-hidden="true" />;
}

const GROUPS = [
  { door: "perform", label: "Perform", icon: Activity },
  { door: "convert", label: "Convert", icon: Filter },
  { door: "create", label: "Create", icon: PenTool },
  { door: "any", label: "Reports", icon: FileText },
] as const;

// Templates are prompts ready to fire. Four sit under the bar as pills;
// "Show more templates" opens the whole catalog, grouped by door, with those
// four among them as full cards.
function Templates({
  templates,
  onAsk,
  open,
  onOpenChange: setOpen,
}: {
  templates: Template[];
  onAsk: (q: string) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!open) {
    return (
      <div className="mx-auto -mt-4 flex w-full max-w-3xl flex-col items-center gap-3">
        <div className="flex flex-wrap justify-center gap-2">
          {templates.slice(0, 4).map((t, i) => (
            <button
              key={t.id}
              type="button"
              title={t.description}
              onClick={() => onAsk(t.prompt)}
              className="flex animate-rise items-center gap-2 border bg-card px-3 py-1.5 text-sm transition-colors hover:border-foreground/40 hover:bg-muted"
              style={{ animationDelay: `${i * 40}ms` }}
            >
              <TemplateGlyph icon={t.icon} className="size-3.5 text-muted-foreground" />
              {t.title}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          Show more templates <ChevronDown className="size-4" />
        </button>
      </div>
    );
  }
  return (
    <div className="animate-rise space-y-6">
      {GROUPS.map((g) => {
        const items = templates.filter((t) => t.door === g.door);
        if (items.length === 0) return null;
        return (
          <section key={g.door}>
            <h2 className="mb-2.5 flex items-center gap-2 font-mono text-[0.65rem] tracking-widest text-muted-foreground uppercase">
              <g.icon className="size-3.5" />
              {g.label}
            </h2>
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {items.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  onClick={() => onAsk(t.prompt)}
                  className="group flex items-start gap-3 border bg-card p-4 text-left transition-colors hover:border-foreground/40"
                >
                  <span className="flex size-8 shrink-0 items-center justify-center bg-muted">
                    <TemplateGlyph icon={t.icon} className="size-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium leading-snug">{t.title}</span>
                    <span className="mt-1 block text-sm text-muted-foreground">
                      {t.description}
                    </span>
                  </span>
                  <ArrowUpRight className="size-4 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                </button>
              ))}
            </div>
          </section>
        );
      })}
      <div className="flex justify-center">
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          Show fewer <ChevronUp className="size-4" />
        </button>
      </div>
    </div>
  );
}
