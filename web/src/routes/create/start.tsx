import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, FileText, Upload } from "lucide-react";
import { type FormEvent, type ReactNode, useId, useState } from "react";
import { type Analyst, useStartSetup } from "@/api/create";
import { useCreateProject } from "@/api/projects";
import { ErrorNote, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusMark } from "@/components/ui/status-chip";
import { cn } from "@/lib/utils";
import { SampleChip } from "./shared";

const ANALYSTS: { id: Analyst; name: string; looks: string }[] = [
  { id: "performance", name: "Performance marketer", looks: "Campaigns, costs and conversion" },
  { id: "media", name: "Media planner", looks: "Channels, budgets and flighting" },
  { id: "brand", name: "Brand strategist", looks: "Positioning, voice and audiences" },
  { id: "seo", name: "SEO specialist", looks: "Search demand and organic reach" },
  { id: "creative", name: "Creative director", looks: "Concepts, formats and testing" },
  { id: "crm", name: "CRM and sales analyst", looks: "Lead quality and follow-up" },
];

const TEXTAREA =
  "w-full min-w-0 resize-y border border-input bg-transparent px-2.5 py-2 text-sm leading-5 outline-none transition-colors placeholder:text-muted-foreground hover:border-ring focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50";

// Starting a brand is the first step of the CEO's flow: give Adx the brand's
// context, say who should look at it and with which skills, and Adx sets up
// the project and works through analysis and research to a first media plan.
// The brand is created for real; the setup run is sample until phase 2.
export function StartBrandPage() {
  const navigate = useNavigate();
  const create = useCreateProject();
  const start = useStartSetup();
  const ids = {
    name: useId(),
    website: useId(),
    elsewhere: useId(),
    brief: useId(),
    results: useId(),
  };
  const [name, setName] = useState("");
  const [hasHistory, setHasHistory] = useState(true);
  const [analysts, setAnalysts] = useState<Analyst[]>(["performance", "media", "brand"]);
  const [skillsFile, setSkillsFile] = useState<string | null>(null);
  const [templateFile, setTemplateFile] = useState<string | null>(null);
  const busy = create.isPending || start.isPending;
  const error = create.error ?? start.error;

  const toggle = (a: Analyst) =>
    setAnalysts((xs) => (xs.includes(a) ? xs.filter((x) => x !== a) : [...xs, a]));

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    const project = await create.mutateAsync({ name: name.trim() });
    await start.mutateAsync({
      projectId: project.id,
      brief: text("brief"),
      results: text("results"),
      website: text("website"),
      elsewhere: text("elsewhere"),
      hasHistory,
      analysts,
      skillsFile,
      templateFile,
    });
    navigate({ to: "/brands/$projectId", params: { projectId: String(project.id) } });
  }

  return (
    <>
      <Link
        to="/brands"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Brands
      </Link>
      <PageHeader
        title="Start a brand"
        subtitle="Tell Adx what the brand is and who should look at it. Adx sets up the brand and works from analysis and research to a first media plan."
      />
      <form
        onSubmit={onSubmit}
        className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]"
      >
        <div className="min-w-0 space-y-6">
          <Panel title="The brand" description="A development or product Reportage markets.">
            <div className="grid gap-4">
              <Field id={ids.name} label="Name">
                <Input
                  id={ids.name}
                  name="name"
                  required
                  maxLength={100}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="The Wadi"
                />
              </Field>
              <Field id={ids.website} label="Website" optional>
                <Input id={ids.website} name="website" type="url" placeholder="https://" />
              </Field>
              <Field
                id={ids.elsewhere}
                label="Where else it lives"
                hint="Social handles, a brochure link, a YouTube channel. One per line."
                optional
              >
                <textarea id={ids.elsewhere} name="elsewhere" rows={3} className={TEXTAREA} />
              </Field>
            </div>
          </Panel>

          <Panel
            title="What Adx should know"
            description="The context you'd give a specialist before they start."
          >
            <div className="grid gap-4">
              <Field
                id={ids.brief}
                label="Brief"
                hint="What it is, who it's for, what's selling and what isn't."
              >
                <textarea
                  id={ids.brief}
                  name="brief"
                  rows={5}
                  className={TEXTAREA}
                  placeholder="Townhouses on the edge of Al Shamkha, 20 minutes from the airport. Families renting in Abu Dhabi are the main buyers; the payment plan is the lever. Cash offers convert well with investors."
                />
              </Field>
              <Field
                id={ids.results}
                label="Results so far"
                hint="Sales, revenue, units sold, what past campaigns achieved."
                optional
              >
                <textarea id={ids.results} name="results" rows={3} className={TEXTAREA} />
              </Field>
            </div>
          </Panel>

          <Panel
            title="History"
            description="Decides whether Adx audits accounts or works from outside data."
          >
            <fieldset className="grid gap-3 sm:grid-cols-2">
              <legend className="sr-only">Has this brand run ads before?</legend>
              <Choice
                checked={hasHistory}
                onSelect={() => setHasHistory(true)}
                title="It has run ads before"
                text="Adx audits past campaigns, spend and leads in the connected accounts."
              />
              <Choice
                checked={!hasHistory}
                onSelect={() => setHasHistory(false)}
                title="It's new"
                text="No history to audit. Adx works from public data and comparable brands."
              />
            </fieldset>
          </Panel>

          <Panel
            title="Who looks at it"
            description="The specialists Adx works as. Each brings its own skills and questions."
          >
            <div className="grid gap-2 sm:grid-cols-2">
              {ANALYSTS.map((a) => {
                const on = analysts.includes(a.id);
                return (
                  <button
                    key={a.id}
                    type="button"
                    aria-pressed={on}
                    onClick={() => toggle(a.id)}
                    className={cn(
                      "flex items-start gap-3 border p-3 text-left transition-colors",
                      on ? "border-foreground bg-card" : "bg-card hover:border-foreground/40",
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        "mt-0.5 size-4 shrink-0 border",
                        on ? "border-foreground bg-foreground" : "border-input",
                      )}
                    />
                    <span className="min-w-0">
                      <span className="block text-sm font-medium">{a.name}</span>
                      <span className="block text-xs text-muted-foreground">{a.looks}</span>
                    </span>
                  </button>
                );
              })}
            </div>
          </Panel>

          <Panel
            title="Skills and template"
            description="Both are optional. Adx uses its own until you add them."
          >
            <div className="grid gap-3 sm:grid-cols-2">
              <FilePick
                label="Marketing skills file"
                accept=".md,.txt"
                file={skillsFile}
                onPick={setSkillsFile}
                empty="Adx's default skills"
              />
              <FilePick
                label="Media-plan template"
                accept=".xlsx,.xls,.csv"
                file={templateFile}
                onPick={setTemplateFile}
                empty="Adx's own plan layout"
              />
            </div>
          </Panel>
        </div>

        <aside className="lg:sticky lg:top-6">
          <Panel
            title={
              <span className="flex items-center gap-2">
                What Adx will do <SampleChip phase={2} />
              </span>
            }
          >
            <ol className="space-y-3">
              {[
                ["Brand analysis", "What it sells, to whom, its voice and gaps"],
                hasHistory
                  ? ["Account audit", "Past campaigns, spend and leads"]
                  : ["Outside audit", "Public data and comparable brands"],
                ["Market research", "Demand, prices and audiences"],
                ["Competitor research", "Their sites, socials and ad libraries"],
                ["SEO and keywords", "Search terms by intent and cost"],
                ["First media plan", templateFile ? `In ${templateFile}` : "In Adx's own layout"],
              ].map(([t, d], i) => (
                <li key={t} className="flex gap-3">
                  <span className="w-4 shrink-0 font-mono text-xs leading-5 text-muted-foreground">
                    {i + 1}
                  </span>
                  <span className="min-w-0">
                    <span className="block text-sm font-medium">{t}</span>
                    <span className="block text-xs text-muted-foreground">{d}</span>
                  </span>
                </li>
              ))}
            </ol>
            <p className="mt-4 flex items-start gap-2 border-t pt-4 text-xs text-muted-foreground">
              <StatusMark status="caution" className="mt-1" />
              Then you approve the plan. Nothing launches or spends before that.
            </p>
            <Button
              type="submit"
              size="lg"
              className="mt-4 w-full"
              disabled={busy || !name.trim() || analysts.length === 0}
            >
              {busy ? "Starting…" : "Start setup"}
            </Button>
            <p className="mt-2 text-xs text-muted-foreground">
              Adds the brand now. The setup run is sample data until phase 2.
            </p>
            {error && (
              <div className="mt-3">
                <ErrorNote error={error} />
              </div>
            )}
          </Panel>
        </aside>
      </form>
    </>
  );
}

function Field({
  id,
  label,
  hint,
  optional,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  optional?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="grid gap-1.5">
      <label htmlFor={id} className="flex items-baseline gap-2 text-sm font-medium">
        {label}
        {optional && <span className="text-xs font-normal text-muted-foreground">Optional</span>}
      </label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function Choice({
  checked,
  onSelect,
  title,
  text,
}: {
  checked: boolean;
  onSelect: () => void;
  title: string;
  text: string;
}) {
  return (
    <label
      className={cn(
        "flex cursor-pointer items-start gap-3 border bg-card p-3 transition-colors",
        checked ? "border-foreground" : "hover:border-foreground/40",
      )}
    >
      <input
        type="radio"
        name="history"
        checked={checked}
        onChange={onSelect}
        className="mt-1 accent-foreground"
      />
      <span>
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs text-muted-foreground">{text}</span>
      </span>
    </label>
  );
}

function FilePick({
  label,
  accept,
  file,
  onPick,
  empty,
}: {
  label: string;
  accept: string;
  file: string | null;
  onPick: (name: string | null) => void;
  empty: string;
}) {
  const id = useId();
  return (
    <div className="flex items-center gap-3 border bg-card p-3">
      <span className="flex size-8 shrink-0 items-center justify-center bg-muted">
        <FileText className="size-4" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium">{label}</span>
        <span className="block truncate text-xs text-muted-foreground">
          {file ?? `Not added · ${empty}`}
        </span>
      </span>
      <label
        htmlFor={id}
        className="flex h-7 shrink-0 cursor-pointer items-center gap-1 border px-2.5 text-[0.8rem] font-medium hover:bg-muted"
      >
        <Upload className="size-3.5" /> {file ? "Change" : "Add"}
      </label>
      <input
        id={id}
        type="file"
        accept={accept}
        className="sr-only"
        onChange={(e) => onPick(e.target.files?.[0]?.name ?? null)}
      />
    </div>
  );
}
