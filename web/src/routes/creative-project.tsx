import { Link, useNavigate } from "@tanstack/react-router";
import { ExternalLink, Link2, Pencil, Plus, Sparkles } from "lucide-react";
import { useState } from "react";
import { useCreateConcept, useGenerateConcepts, useResolveComment } from "@/api/creative";
import type { Ad, Comment, Concept, CreativeProject } from "@/api/types.gen";
import { FieldError, Form } from "@/components/form";
import { EmptyState, Eyebrow, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { fileUrl } from "@/creative/files";
import { FORMATS } from "@/creative/naming";
import { ReviewState } from "@/creative/ui";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";

export function reviewLink(key: string) {
  return `${window.location.origin}${import.meta.env.BASE_URL.replace(/\/$/, "")}/review/${key}`;
}

export function ReviewLinkButtons({ settings }: { settings: CreativeProject["settings"] }) {
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

export function ConceptsTab({ project, canEdit }: { project: CreativeProject; canEdit: boolean }) {
  const projectId = project.settings.projectId;
  return (
    <div className="space-y-8">
      {canEdit && (
        <div className="flex flex-wrap items-start gap-x-8 gap-y-4">
          <NewConceptForm projectId={projectId} />
          <AskForConcepts projectId={projectId} />
        </div>
      )}
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

export function NewConceptForm({
  projectId,
  campaignId = 0,
}: {
  projectId: number;
  campaignId?: number;
}) {
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const create = useCreateConcept(projectId);
  const navigate = useNavigate();
  return (
    <Form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { code, name, campaignId },
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
          data-error="Give the idea a code, like C1."
        />
        <FieldError htmlFor="concept-code" />
      </div>
      <Input
        aria-label="Concept name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Cash offer"
        className="w-56"
      />
      <Button type="submit" variant="outline" disabled={create.isPending}>
        <Plus />
        Add
      </Button>
      {create.error && <p className="w-full text-sm text-destructive">{create.error.message}</p>}
    </Form>
  );
}

// Adx proposes concepts from the brand's approved facts and current
// offers. They land as ordinary concepts, to edit or delete like any other.
export function AskForConcepts({
  projectId,
  campaignId = 0,
}: {
  projectId: number;
  campaignId?: number;
}) {
  const [instructions, setInstructions] = useState("");
  const [count, setCount] = useState(3);
  const generate = useGenerateConcepts(projectId);
  return (
    <Form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        generate.mutate({ count, instructions, campaignId });
      }}
    >
      <div className="flex flex-col gap-1.5">
        <label htmlFor="concept-ask" className="text-sm font-medium">
          Ask Adx for concepts
        </label>
        <Input
          id="concept-ask"
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
          placeholder="Angle, audience, anything to avoid"
          className="w-72"
        />
      </div>
      <Input
        aria-label="How many"
        type="number"
        min={1}
        max={6}
        value={count}
        onChange={(e) => setCount(Number(e.target.value))}
        className="w-16"
      />
      <Button type="submit" disabled={generate.isPending}>
        <Sparkles />
        {generate.isPending ? "Writing" : "Write"}
      </Button>
      {generate.error && (
        <p className="w-full text-sm text-destructive">{generate.error.message}</p>
      )}
      {generate.data && (
        <div className="w-full text-sm text-muted-foreground">
          <p>
            {generate.data.concepts.length === 0
              ? "Nothing worth saving came back."
              : `${generate.data.concepts.length} ${generate.data.concepts.length === 1 ? "concept" : "concepts"} added from the approved facts.`}
          </p>
          {generate.data.notes.map((n) => (
            <p key={n}>{n}</p>
          ))}
        </div>
      )}
    </Form>
  );
}

export function ConceptSection({
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
  const video = ad.kind === "video";
  return (
    <li className="min-w-0 space-y-1.5">
      <div className="relative grid h-56 place-items-center border bg-muted p-2">
        {ad.current && video ? (
          <video
            src={fileUrl(ad.current.file)}
            poster={fileUrl(ad.current.preview)}
            controls
            muted
            playsInline
            preload="none"
            className="max-h-52 max-w-full"
            style={{ aspectRatio: `${f.width} / ${f.height}` }}
          />
        ) : ad.current ? (
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
            {video ? "Being made" : "Not published yet"}
          </span>
        )}
      </div>
      <div className="flex items-center justify-between gap-2">
        <ReviewState status={ad.status} />
        {canEdit && !video && (
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
        {video ? "Video" : ad.fmt} ·{" "}
        {video
          ? ad.current?.method === "model"
            ? "Veo"
            : "Motion"
          : ad.hasCta
            ? "With CTA"
            : "No CTA"}
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
