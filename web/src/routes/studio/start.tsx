import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, FileText, Upload } from "lucide-react";
import { type FormEvent, type ReactNode, useId, useState } from "react";
import { useAssignProject, useBrands, useCreateBrand } from "@/api/brands";
import type { Analyst } from "@/api/create";
import { useCreateBrief } from "@/api/research";
import { ErrorNote, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusMark } from "@/components/ui/status-chip";
import { postJSON } from "@/engine/api/client";
import { useCreateProject } from "@/engine/api/projects";
import { cn } from "@/lib/utils";

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
// Everything here is real: the brand, its first project, and a research brief
// that reads the website and the competitors named, audits the account when
// there is history, and starts running at once. The specialists and the
// skills file are recorded in the brand's notes until the engine has a place
// for them (B5.2's registry).
export function StartBrandPage() {
  const navigate = useNavigate();
  const brands = useBrands();
  // Our side has one developer; a new project goes under it. Only the very
  // first project also names the developer.
  const developer = brands.data?.brands.find((b) => b.kind === "own") ?? null;
  const createBrand = useCreateBrand();
  const createProject = useCreateProject();
  const assign = useAssignProject();
  const createBrief = useCreateBrief();

  const ids = {
    name: useId(),
    project: useId(),
    competitors: useId(),
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
  const [project, setProject] = useState("");
  const busy =
    createBrand.isPending || createProject.isPending || assign.isPending || createBrief.isPending;
  const error = createBrand.error ?? createProject.error ?? assign.error ?? createBrief.error;

  const toggle = (a: Analyst) =>
    setAnalysts((xs) => (xs.includes(a) ? xs.filter((x) => x !== a) : [...xs, a]));

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    const lines = (k: string) =>
      text(k)
        .split("\n")
        .map((l) => l.trim())
        .filter(Boolean);
    const notes = [
      text("brief"),
      text("results") && `Results so far: ${text("results")}`,
      `Specialists: ${analysts.map((a) => ANALYSTS.find((x) => x.id === a)?.name ?? a).join(", ")}`,
      skillsFile && `Skills file: ${skillsFile}`,
      templateFile && `Media-plan template: ${templateFile}`,
    ]
      .filter(Boolean)
      .join("\n\n");
    const brand =
      developer ?? (await createBrand.mutateAsync({ name: name.trim(), kind: "own", notes }));
    const p = await createProject.mutateAsync({ name: project.trim() || name.trim() });
    await assign.mutateAsync({ projectId: p.id, brandId: brand.id });
    const website = text("website");
    const competitors = lines("competitors").map((l) => {
      const [cname, ...urls] = l.split(/\s+/);
      return { name: cname ?? l, urls: urls.filter((u) => /^https?:\/\//.test(u)) };
    });
    const brief = await createBrief.mutateAsync({
      brandId: brand.id,
      projectId: p.id,
      title: `${p.name}: setup research`,
      question: `Who is ${brand.name} selling ${p.name} to, who else is selling to them, and what will it cost to reach them?${developer && notes ? `\n\n${notes}` : ""}`,
      geography: "AE",
      language: "en",
      urls: [website, ...lines("elsewhere")].filter((u) => /^https?:\/\//.test(u)),
      competitors,
      keywordSeeds: [],
      googleAdsAccountId: 0,
      auditAccount: hasHistory,
    });
    // Start the run and move on; the brand page shows items as they're fetched.
    await postJSON(`/api/research/briefs/${brief.brief.id}/runs`, {});
    navigate({ to: "/projects/$projectId", params: { projectId: String(p.id) } });
  }

  return (
    <>
      <Link
        to="/projects"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Projects
      </Link>
      <PageHeader
        title="New project"
        subtitle="Tell Adx what the development is and who should look at it. Adx sets it up and works from analysis and research to a first media plan."
      />
      <form
        onSubmit={onSubmit}
        className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]"
      >
        <div className="min-w-0 space-y-6">
          <Panel
            title="The project"
            description={
              developer
                ? `A development ${developer.name} is selling.`
                : "The development Adx markets first, and the developer behind it."
            }
          >
            <div className="grid gap-4">
              {!developer && (
                <Field id={ids.name} label="Developer">
                  <Input
                    id={ids.name}
                    name="name"
                    required
                    maxLength={100}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Reportage Properties"
                  />
                </Field>
              )}
              <Field
                id={ids.project}
                label="Project"
                hint="The development's name, as buyers know it."
              >
                <Input
                  id={ids.project}
                  name="project"
                  required
                  maxLength={100}
                  value={project}
                  onChange={(e) => setProject(e.target.value)}
                  placeholder="The Wadi"
                />
              </Field>
              <Field id={ids.website} label="Website" optional>
                <Input id={ids.website} name="website" type="url" placeholder="https://" />
              </Field>
              <Field
                id={ids.elsewhere}
                label="Pages to read"
                hint="Project pages, a brochure link, a listing. One URL per line; Adx reads each one."
                optional
              >
                <textarea id={ids.elsewhere} name="elsewhere" rows={3} className={TEXTAREA} />
              </Field>
              <Field
                id={ids.competitors}
                label="Competitors"
                hint="One per line: a name, then their page URLs. Adx reads their pages and keeps them on the competitor side."
                optional
              >
                <textarea
                  id={ids.competitors}
                  name="competitors"
                  rows={3}
                  className={TEXTAREA}
                  placeholder="Aldar https://www.aldar.com/en/explore-aldar/yas-park-gate"
                />
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
          <Panel title={<span className="flex items-center gap-2">What Adx will do</span>}>
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
              disabled={busy || !name.trim() || !project.trim() || analysts.length === 0}
            >
              {busy ? "Starting…" : "Start setup"}
            </Button>
            <p className="mt-2 text-xs text-muted-foreground">
              The research starts the moment you press this. Keyword ideas and the audit wait for
              connected accounts.
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
