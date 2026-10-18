import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { Download, ExternalLink, Link2, Pencil, Plus, Upload } from "lucide-react";
import { useState } from "react";
import { useCreateConcept, useCreativeProject, useResolveComment } from "@/api/creative";
import type { Ad, Comment, Concept, CreativeProject } from "@/api/types.legacy";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { BrandKitForm } from "@/creative/brand-kit-form";
import { fileUrl } from "@/creative/files";
import { FORMATS } from "@/creative/naming";
import { RendersTab } from "@/creative/renders-tab";
import { SettingsTab } from "@/creative/settings-tab";
import { ReviewState, ReviewTally } from "@/creative/ui";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";

export type CreativeTab = "concepts" | "renders" | "brand" | "settings";

export function creativeTabSearch(raw: Record<string, unknown>): { tab?: CreativeTab } {
  const t = raw.tab;
  return t === "renders" || t === "brand" || t === "settings" ? { tab: t } : {};
}

export function reviewLink(key: string) {
  return `${window.location.origin}${import.meta.env.BASE_URL.replace(/\/$/, "")}/review/${key}`;
}

export function CreativeProjectPage({ projectId }: { projectId: number }) {
  const project = useCreativeProject(projectId);
  const { tab = "concepts" } = useSearch({ strict: false }) as { tab?: CreativeTab };
  const navigate = useNavigate();
  const session = useSession();
  const canEdit = session.role !== "viewer";

  if (project.isPending) return <LoadingRows rows={8} />;
  if (project.error) return <ErrorNote error={project.error} />;
  const p = project.data;
  const ads = p.concepts.flatMap((c) => c.ads);
  const count = (s: Ad["status"]) => ads.filter((a) => a.status === s).length;

  return (
    <section>
      <Link to="/creative" className="text-sm text-muted-foreground hover:text-foreground">
        ← Creatives
      </Link>
      <PageHeader
        className="mt-2"
        title={p.settings.projectName}
        subtitle={
          <span className="flex flex-wrap items-center gap-x-4 gap-y-1">
            <span>
              Prefix <span className="font-mono">{p.settings.prefix}</span> · {p.concepts.length}{" "}
              {p.concepts.length === 1 ? "concept" : "concepts"} · {ads.length}{" "}
              {ads.length === 1 ? "ad" : "ads"}
            </span>
            <ReviewTally
              approved={count("approved")}
              revise={count("revise")}
              waiting={count("waiting")}
            />
          </span>
        }
        actions={
          <>
            <ReviewLinkButtons settings={p.settings} />
            <a
              href={`/api/creative/projects/${projectId}/download?set=approved`}
              className={buttonVariants({ variant: "outline" })}
            >
              <Download />
              Approved
            </a>
            {canEdit && (
              <Link
                to="/creative/$projectId/upload"
                params={{ projectId: String(projectId) }}
                className={buttonVariants()}
              >
                <Upload />
                Upload ads
              </Link>
            )}
          </>
        }
      />
      <Segmented
        label="Project sections"
        value={tab}
        onChange={(t) =>
          navigate({
            to: "/creative/$projectId",
            params: { projectId: String(projectId) },
            search: t === "concepts" ? {} : { tab: t },
            replace: true,
          })
        }
        options={[
          { value: "concepts", label: "Concepts", count: p.concepts.length },
          { value: "renders", label: "Renders", count: p.renders.filter((r) => !r.hidden).length },
          { value: "brand", label: "Brand kit" },
          { value: "settings", label: "Settings" },
        ]}
        className="mb-6"
      />
      {tab === "concepts" && <ConceptsTab project={p} canEdit={canEdit} />}
      {tab === "renders" && <RendersTab project={p} canEdit={canEdit} />}
      {tab === "brand" && <BrandKitForm settings={p.settings} canEdit={canEdit} />}
      {tab === "settings" && <SettingsTab settings={p.settings} canEdit={canEdit} />}
    </section>
  );
}

function ReviewLinkButtons({ settings }: { settings: CreativeProject["settings"] }) {
  const [copied, setCopied] = useState(false);
  const link = reviewLink(settings.reviewKey);
  if (!settings.reviewActive) {
    return <span className="text-sm text-muted-foreground">Review link paused</span>;
  }
  return (
    <>
      <Button
        variant="outline"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(link);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
          } catch {
            window.prompt("Copy the review link", link);
          }
        }}
      >
        <Link2 />
        {copied ? "Copied" : "Copy review link"}
      </Button>
      <a
        href={link}
        target="_blank"
        rel="noreferrer"
        className={buttonVariants({ variant: "outline", size: "icon" })}
        aria-label="Open the review board"
        title="Open the review board"
      >
        <ExternalLink />
      </a>
    </>
  );
}

function ConceptsTab({ project, canEdit }: { project: CreativeProject; canEdit: boolean }) {
  const projectId = project.settings.projectId;
  return (
    <div className="space-y-8">
      {canEdit && <NewConceptForm projectId={projectId} />}
      {project.concepts.length === 0 ? (
        <Panel>
          <EmptyState title="No concepts yet">
            {canEdit
              ? "Upload finished ads and their concepts are made from the file names, or add a concept and build its ads from copy."
              : "Someone on the marketing team adds them."}
          </EmptyState>
        </Panel>
      ) : (
        project.concepts.map((c) => (
          <ConceptSection key={c.id} concept={c} projectId={projectId} canEdit={canEdit} />
        ))
      )}
      <CommentsPanel project={project} canEdit={canEdit} />
    </div>
  );
}

function NewConceptForm({ projectId }: { projectId: number }) {
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const create = useCreateConcept(projectId);
  const navigate = useNavigate();
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { code, name },
          {
            onSuccess: (c) =>
              navigate({
                to: "/creative/$projectId/concepts/$conceptId",
                params: { projectId: String(projectId), conceptId: String(c.id) },
              }),
          },
        );
      }}
    >
      <div className="flex flex-col gap-1.5">
        <label htmlFor="concept-code" className="text-sm font-medium">
          New concept
        </label>
        <Input
          id="concept-code"
          value={code}
          onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder="C1"
          className="w-24 font-mono"
          required
        />
      </div>
      <Input
        aria-label="Concept name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Cash offer"
        className="w-56"
      />
      <Button type="submit" variant="outline" disabled={create.isPending || !code}>
        <Plus />
        Add
      </Button>
      {create.error && <p className="w-full text-sm text-destructive">{create.error.message}</p>}
    </form>
  );
}

function ConceptSection({
  concept,
  projectId,
  canEdit,
}: {
  concept: Concept;
  projectId: number;
  canEdit: boolean;
}) {
  return (
    <section className="space-y-3">
      <header className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 className="text-lg font-semibold">
          <span className="font-mono text-muted-foreground">{concept.code}</span> {concept.name}
        </h2>
        <span className="text-sm text-muted-foreground">
          {concept.ads.length} {concept.ads.length === 1 ? "ad" : "ads"}
        </span>
        {canEdit && (
          <Link
            to="/creative/$projectId/concepts/$conceptId"
            params={{ projectId: String(projectId), conceptId: String(concept.id) }}
            className="text-sm underline-offset-4 hover:underline"
          >
            Copy and generate
          </Link>
        )}
      </header>
      {concept.why && <p className="max-w-3xl text-sm text-muted-foreground">{concept.why}</p>}
      {concept.ads.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No ads yet. Write the copy and generate them, or upload them.
        </p>
      ) : (
        <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5 xl:grid-cols-6">
          {concept.ads.map((a) => (
            <AdTile key={a.id} ad={a} projectId={projectId} canEdit={canEdit} />
          ))}
        </ul>
      )}
    </section>
  );
}

function AdTile({ ad, projectId, canEdit }: { ad: Ad; projectId: number; canEdit: boolean }) {
  const f = FORMATS[ad.fmt];
  return (
    <li className="min-w-0 space-y-1.5">
      <div className="relative grid h-56 place-items-center border bg-muted p-2">
        {ad.current ? (
          <a href={fileUrl(ad.current.file)} target="_blank" rel="noreferrer">
            <img
              src={fileUrl(ad.current.preview)}
              alt={ad.code}
              loading="lazy"
              className="max-h-52 max-w-full object-contain"
              style={{ aspectRatio: `${f.width} / ${f.height}` }}
            />
          </a>
        ) : (
          <span className="absolute inset-0 grid place-items-center p-2 text-center text-xs text-muted-foreground">
            Not published yet
          </span>
        )}
      </div>
      <div className="flex items-center justify-between gap-2">
        <ReviewState status={ad.status} />
        {canEdit && (
          <Link
            to="/creative/$projectId/ads/$adId/edit"
            params={{ projectId: String(projectId), adId: String(ad.id) }}
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
          >
            <Pencil className="size-3" />
            Edit
          </Link>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {ad.fmt} · {ad.hasCta ? "With CTA" : "No CTA"}
        {ad.lang === "ar" && " · Arabic"}
        {ad.current && ad.current.version > 1 && ` · v${ad.current.version}`}
        {ad.comments > 0 && ` · ${ad.comments} ${ad.comments === 1 ? "comment" : "comments"}`}
      </p>
      <p className="truncate font-mono text-[0.68rem] text-muted-foreground" title={ad.fileName}>
        {ad.fileName}
      </p>
    </li>
  );
}

// Every comment from the review board, newest first, so the team can work
// through them and mark each one done.
function CommentsPanel({ project, canEdit }: { project: CreativeProject; canEdit: boolean }) {
  const [show, setShow] = useState<"open" | "all">("open");
  const resolve = useResolveComment();
  const comments = [...project.comments].reverse();
  const open = comments.filter((c) => !c.resolved);
  const list = show === "open" ? open : comments;
  if (comments.length === 0) return null;
  return (
    <Panel
      title="Comments from the review board"
      description="Newest first. Mark one done once the change is made."
      action={
        <Segmented
          label="Which comments"
          value={show}
          onChange={setShow}
          options={[
            { value: "open", label: "Open", count: open.length },
            { value: "all", label: "All", count: comments.length },
          ]}
        />
      }
      bodyClassName="p-0"
    >
      {list.length === 0 ? (
        <EmptyState title="Nothing open">Every comment is marked done.</EmptyState>
      ) : (
        <ul className="divide-y">
          {list.map((c) => (
            <CommentRow
              key={c.id}
              comment={c}
              canEdit={canEdit}
              busy={resolve.isPending}
              onResolve={(resolved) => resolve.mutate({ id: c.id, resolved })}
            />
          ))}
        </ul>
      )}
    </Panel>
  );
}

function CommentRow({
  comment,
  canEdit,
  busy,
  onResolve,
}: {
  comment: Comment;
  canEdit: boolean;
  busy: boolean;
  onResolve: (resolved: boolean) => void;
}) {
  return (
    <li className={cn("flex items-start gap-4 px-5 py-3", comment.resolved && "opacity-60")}>
      <div className="min-w-0 flex-1 space-y-1">
        <Eyebrow>
          {comment.ad} · v{comment.version}
          {comment.x !== null && " · pinned"}
        </Eyebrow>
        <p className="text-sm whitespace-pre-wrap">{comment.body}</p>
        <p className="text-xs text-muted-foreground">
          {comment.name} ·{" "}
          {formatDate(comment.createdAt, {
            day: "numeric",
            month: "short",
            hour: "2-digit",
            minute: "2-digit",
          })}
        </p>
      </div>
      {canEdit && (
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={() => onResolve(!comment.resolved)}
        >
          {comment.resolved ? "Reopen" : "Done"}
        </Button>
      )}
    </li>
  );
}
