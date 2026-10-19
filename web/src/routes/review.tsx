import { useEffect, useMemo, useRef, useState } from "react";
import { useReviewBoard, useReviewComment, useReviewDecide, useReviewPick } from "@/api/creative";
import type {
  Board,
  BoardAd,
  BoardConcept,
  Comment,
  Decision,
  ReviewStatus,
} from "@/api/types.gen";
import { AdxWordmark } from "@/components/brand";
import { Segmented } from "@/components/segmented";
import { normalizeKit } from "@/creative/brand";
import { fileUrl } from "@/creative/files";
import { REVIEW_LABEL, ReviewState, ReviewTally, textareaClass } from "@/creative/ui";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";

const NAME_KEY = "adx-reviewer";

function storedName() {
  try {
    return (localStorage.getItem(NAME_KEY) ?? "").slice(0, 60);
  } catch {
    return "";
  }
}

const when = (iso: string) =>
  formatDate(iso, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

// The public review board. Reviewers open it from a link, without an
// account, type their name once, and approve, ask for changes or comment on
// each ad. Everything posted is shared with everyone who has the link.
export function ReviewPage({ reviewKey }: { reviewKey: string }) {
  const board = useReviewBoard(reviewKey);
  useEffect(() => {
    document.title = board.data
      ? `${board.data.projectName} review | Adx`
      : "Creative review | Adx";
  }, [board.data]);

  if (board.isPending) return <State title="Loading the review board" text="One moment." />;
  if (board.error) {
    const missing = "status" in board.error && board.error.status === 404;
    return missing ? (
      <State
        title="This review link isn't active"
        text="Ask the person who sent it for the current link."
      />
    ) : (
      <State
        title="The review board didn't load"
        text="Check your connection and reload the page."
      />
    );
  }
  return <BoardView board={board.data} reviewKey={reviewKey} stale={board.isRefetchError} />;
}

function State({ title, text }: { title: string; text: string }) {
  return (
    <main className="grid min-h-screen place-items-center bg-background px-6 text-center">
      <div className="max-w-md space-y-3">
        <h1 className="text-3xl font-semibold">{title}</h1>
        <p className="text-muted-foreground">{text}</p>
      </div>
    </main>
  );
}

type Sheet = "ads" | "renders";
type Filter = "all" | ReviewStatus;
type FmtFilter = "all" | BoardAd["fmt"];

function BoardView({
  board,
  reviewKey,
  stale,
}: {
  board: Board;
  reviewKey: string;
  stale: boolean;
}) {
  const [name, setName] = useState(storedName);
  const [needName, setNeedName] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [sheet, setSheet] = useState<Sheet>("ads");
  const [status, setStatus] = useState<Filter>("all");
  const [fmt, setFmt] = useState<FmtFilter>("all");
  const [zones, setZones] = useState(false);
  const [lightbox, setLightbox] = useState<{ src: string; alt: string } | null>(null);
  const [pickFor, setPickFor] = useState("");
  const nameBox = useRef<HTMLInputElement>(null);
  const kit = useMemo(() => normalizeKit(board.brandKit), [board.brandKit]);
  const logo = kit.logos.onDark;

  const decide = useReviewDecide(reviewKey);
  const comment = useReviewComment(reviewKey);
  const pick = useReviewPick(reviewKey);

  const me = name.replace(/\s+/g, " ").trim().slice(0, 60);
  const askName = () => {
    if (me) return false;
    setNeedName(true);
    nameBox.current?.scrollIntoView({ block: "center" });
    nameBox.current?.focus({ preventScroll: true });
    return true;
  };
  const onName = (v: string) => {
    setName(v);
    setNeedName(false);
    try {
      localStorage.setItem(NAME_KEY, v.replace(/\s+/g, " ").trim().slice(0, 60));
    } catch {
      // A private window: the name lasts for this visit only.
    }
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setLightbox(null);
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const ads = board.concepts.flatMap((c) => c.ads);
  const count = (s: ReviewStatus) => ads.filter((a) => a.status === s).length;
  const byAd = useMemo(() => {
    const m = new Map<string, Comment[]>();
    for (const c of board.comments) m.set(c.ad, [...(m.get(c.ad) ?? []), c]);
    return m;
  }, [board.comments]);
  const votesByAd = useMemo(() => {
    const m = new Map<string, Decision[]>();
    for (const d of board.decisions) m.set(d.ad, [...(m.get(d.ad) ?? []), d]);
    return m;
  }, [board.decisions]);
  const visible = (a: BoardAd) =>
    (status === "all" || a.status === status) && (fmt === "all" || a.fmt === fmt);
  const shown = ads.filter(visible).length;
  const names = new Map(
    board.concepts.flatMap((c) =>
      c.ads.map((a) => [a.code, `${c.code} ${c.name}, ${a.fmt}`] as const),
    ),
  );

  const fail = (what: string) => (e: Error) => setNotice(`${what} wasn't saved. ${e.message}`);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b bg-card">
        <div className="mx-auto flex max-w-7xl flex-wrap items-end gap-x-10 gap-y-6 px-5 pb-8">
          <div className="bg-neutral-900 px-7 pt-8 pb-6">
            {logo ? (
              <img
                src={fileUrl(logo.sha256)}
                alt={board.projectName}
                className="block w-44 max-w-full"
              />
            ) : (
              <span className="block text-2xl font-semibold text-white">{board.projectName}</span>
            )}
          </div>
          <div className="min-w-0 flex-[1_1_22rem] pt-8">
            {board.kicker && (
              <p className="font-mono text-xs tracking-widest text-muted-foreground uppercase">
                {board.kicker}
              </p>
            )}
            <h1 className="mt-2 text-4xl font-semibold sm:text-5xl">Creative review</h1>
            <p className="mt-3 max-w-[60ch] text-sm text-muted-foreground">
              {ads.length} ads across {board.concepts.length} concepts, shown as they'd look in the
              feed and in stories. Add your name once, then approve each ad or ask for changes, and
              leave a comment under any of them. What you post is saved for everyone with this link.
            </p>
          </div>
          <div className="flex-[1_1_16rem] space-y-3">
            <ReviewTally
              approved={count("approved")}
              revise={count("revise")}
              waiting={count("waiting")}
              className="text-sm"
            />
            <div
              className="flex h-2 border bg-muted"
              role="img"
              aria-label="Share of ads approved and sent back"
            >
              <i
                className="block bg-status-ready"
                style={{ width: `${ads.length ? (count("approved") / ads.length) * 100 : 0}%` }}
              />
              <i
                className="block bg-status-caution"
                style={{ width: `${ads.length ? (count("revise") / ads.length) * 100 : 0}%` }}
              />
            </div>
          </div>
        </div>
      </header>

      <section aria-label="Your name" className="border-b">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-4 gap-y-2 px-5 py-3">
          <label htmlFor="reviewer" className="font-mono text-xs tracking-widest uppercase">
            Reviewing as
          </label>
          <input
            id="reviewer"
            ref={nameBox}
            value={name}
            maxLength={60}
            autoComplete="name"
            placeholder="Your name"
            onChange={(e) => onName(e.target.value)}
            className={cn(
              "h-9 w-64 max-w-full border bg-transparent px-3 text-sm outline-none focus-visible:border-foreground",
              needName && "border-status-error",
            )}
          />
          <span className="text-xs text-muted-foreground">
            {needName
              ? "Add your name first, then press the button again."
              : "Your name shows next to your decisions and comments."}
          </span>
          {stale && (
            <span className="ml-auto text-xs text-muted-foreground">
              Couldn't refresh. What you see may be out of date.
            </span>
          )}
        </div>
      </section>

      {notice && (
        <div role="status" className="border-b bg-muted">
          <p className="mx-auto max-w-7xl px-5 py-2 text-sm">{notice}</p>
        </div>
      )}

      <nav
        aria-label="Filters"
        className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur"
      >
        <div className="mx-auto flex max-w-7xl flex-nowrap items-center gap-3 overflow-x-auto px-5 py-2.5 lg:flex-wrap">
          <Segmented
            label="Sheet"
            value={sheet}
            onChange={setSheet}
            options={[
              { value: "ads", label: "Ads", count: ads.length },
              { value: "renders", label: "Renders", count: board.renders.length },
            ]}
          />
          {sheet === "ads" && (
            <>
              <Segmented
                label="Decision"
                value={status}
                onChange={setStatus}
                options={[
                  { value: "all", label: "All" },
                  { value: "waiting", label: "Waiting", tone: "pending" },
                  { value: "approved", label: "Approved", tone: "ready" },
                  { value: "revise", label: "Needs changes", tone: "caution" },
                ]}
              />
              <Segmented
                label="Size"
                value={fmt}
                onChange={setFmt}
                options={[
                  { value: "all", label: "All sizes" },
                  { value: "4:5", label: "4:5" },
                  { value: "1:1", label: "1:1" },
                  { value: "1.91:1", label: "1.91:1" },
                  { value: "9:16", label: "9:16" },
                ]}
              />
              <label className="ml-auto flex shrink-0 items-center gap-2 text-xs whitespace-nowrap">
                <input
                  type="checkbox"
                  checked={zones}
                  onChange={(e) => setZones(e.target.checked)}
                />
                Show the 9:16 keep-clear zones
              </label>
            </>
          )}
        </div>
      </nav>

      <main className="mx-auto max-w-7xl px-5 pb-10">
        {sheet === "ads" &&
          board.concepts.map((c) => {
            const list = c.ads.filter(visible);
            if (list.length === 0) return null;
            const chosen = board.picks.find((p) => p.concept === c.code);
            return (
              <section key={c.code} className="border-b py-9 last:border-b-0">
                <header className="mb-6 flex flex-wrap items-baseline gap-x-8 gap-y-2">
                  <div>
                    <p className="font-mono text-xs tracking-widest text-muted-foreground uppercase">
                      {c.code}
                    </p>
                    <h2 className="text-3xl font-semibold">{c.name}</h2>
                  </div>
                  {c.why && (
                    <p className="max-w-[62ch] flex-[1_1_20rem] text-sm text-muted-foreground">
                      {c.why}
                    </p>
                  )}
                  {board.renders.length > 0 && (
                    <div className="flex w-full flex-wrap items-center gap-3 text-sm">
                      {chosen ? (
                        <span>
                          Render chosen: <b>{chosen.render}</b> by {chosen.name},{" "}
                          {when(chosen.createdAt)}
                        </span>
                      ) : (
                        <span className="text-muted-foreground">
                          No render chosen for this concept.
                        </span>
                      )}
                      <button
                        type="button"
                        className="border px-3 py-1.5 text-xs hover:bg-muted"
                        onClick={() => {
                          setPickFor(c.code);
                          setSheet("renders");
                          window.scrollTo({ top: 0 });
                        }}
                      >
                        {chosen ? "Change the render" : "Choose a render"}
                      </button>
                      {chosen && (
                        <button
                          type="button"
                          className="text-xs underline-offset-4 hover:underline"
                          onClick={() => {
                            if (askName()) return;
                            pick.mutate(
                              { concept: c.code, render: "", name: me },
                              { onError: fail("That choice") },
                            );
                          }}
                        >
                          Clear the choice
                        </button>
                      )}
                    </div>
                  )}
                </header>
                <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,18rem),1fr))] items-start gap-x-6 gap-y-10">
                  {list.map((a) => (
                    <AdUnit
                      key={a.code}
                      concept={c}
                      ad={a}
                      page={board.pageName}
                      zones={zones}
                      me={me}
                      votes={votesByAd.get(a.code) ?? []}
                      comments={byAd.get(a.code) ?? []}
                      busy={decide.isPending || comment.isPending}
                      onOpen={(src, alt) => setLightbox({ src, alt })}
                      onDecide={(s) => {
                        if (askName()) return;
                        setNotice(null);
                        decide.mutate(
                          { ad: a.code, name: me, status: s },
                          { onError: fail("That decision") },
                        );
                      }}
                      onComment={async (body, pin) => {
                        if (askName()) return false;
                        setNotice(null);
                        try {
                          await comment.mutateAsync({
                            ad: a.code,
                            name: me,
                            body,
                            x: pin?.x ?? null,
                            y: pin?.y ?? null,
                          });
                          return true;
                        } catch (e) {
                          fail("That comment")(e as Error);
                          return false;
                        }
                      }}
                    />
                  ))}
                </div>
                {(c.primaryText || c.headline) && (
                  <dl className="mt-8 grid max-w-3xl gap-2 border-t pt-4 text-sm">
                    {(
                      [
                        ["Text", c.primaryText],
                        ["Headline", c.headline],
                        ["Description", c.description],
                        ["Button", c.cta],
                      ] as const
                    ).map(([k, v]) =>
                      v ? (
                        <div key={k} className="grid grid-cols-[6rem_1fr] gap-3">
                          <dt className="font-mono text-xs tracking-widest text-muted-foreground uppercase">
                            {k}
                          </dt>
                          <dd className="whitespace-pre-wrap">{v}</dd>
                        </div>
                      ) : null,
                    )}
                  </dl>
                )}
              </section>
            );
          })}
        {sheet === "ads" && shown === 0 && (
          <p className="py-16 text-center text-muted-foreground">No ads match these filters.</p>
        )}
        {sheet === "renders" && (
          <RendersSheet
            board={board}
            pickFor={pickFor}
            onDone={() => setPickFor("")}
            onOpen={(src, alt) => setLightbox({ src, alt })}
            onPick={(concept, render) => {
              if (askName()) return;
              pick.mutate({ concept, render, name: me }, { onError: fail("That choice") });
            }}
          />
        )}

        <section className="border-t py-9">
          <h2 className="text-2xl font-semibold">All comments</h2>
          <p className="mt-2 mb-5 max-w-[66ch] text-sm text-muted-foreground">
            Every comment, newest first.
          </p>
          {board.comments.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No comments yet. Add one under any ad above.
            </p>
          ) : (
            <ol className="max-w-3xl space-y-2">
              {[...board.comments].reverse().map((c) => (
                <li key={c.id} className={cn("border px-4 py-3", c.resolved && "opacity-60")}>
                  <p className="text-xs text-muted-foreground">
                    <span className="text-foreground">{names.get(c.ad) ?? c.ad}</span> · {c.name} ·{" "}
                    {when(c.createdAt)}
                    {c.resolved && " · done"}
                  </p>
                  <p className="mt-1 text-sm whitespace-pre-wrap">{c.body}</p>
                </li>
              ))}
            </ol>
          )}
        </section>
      </main>

      <footer className="border-t">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-5 py-5 text-xs text-muted-foreground">
          <p>
            For the {board.pageName} review group only. Please don't forward the link. Names are
            typed by each reviewer and aren't checked.
          </p>
          <AdxWordmark />
        </div>
      </footer>

      {lightbox && (
        // biome-ignore lint/a11y/useKeyWithClickEvents: Escape closes it, and so does the Close button
        // biome-ignore lint/a11y/noStaticElementInteractions: clicking the backdrop closes it
        <div
          className="fixed inset-0 z-50 grid place-items-center bg-black/90 p-5"
          onClick={() => setLightbox(null)}
        >
          <img
            src={lightbox.src}
            alt={lightbox.alt}
            className="max-h-full max-w-full object-contain"
          />
          <button
            type="button"
            onClick={() => setLightbox(null)}
            className="absolute top-4 right-4 border border-white/40 bg-black px-3 py-2 text-sm text-white"
          >
            Close
          </button>
        </div>
      )}
    </div>
  );
}

type Pin = { x: number; y: number };

function AdUnit({
  concept,
  ad,
  page,
  zones,
  me,
  votes,
  comments,
  busy,
  onOpen,
  onDecide,
  onComment,
}: {
  concept: BoardConcept;
  ad: BoardAd;
  page: string;
  zones: boolean;
  me: string;
  votes: Decision[];
  comments: Comment[];
  busy: boolean;
  onOpen: (src: string, alt: string) => void;
  onDecide: (s: "approved" | "revise" | "clear") => void;
  onComment: (body: string, pin: Pin | null) => Promise<boolean>;
}) {
  const [draft, setDraft] = useState("");
  const [hint, setHint] = useState("");
  const [placing, setPlacing] = useState(false);
  const [pending, setPending] = useState<Pin | null>(null);
  const [hot, setHot] = useState<number | null>(null);
  const box = useRef<HTMLTextAreaElement>(null);
  const mine = votes.find((v) => v.name.toLowerCase() === me.toLowerCase());
  // Pins belong to the version they were left on.
  const pinned = comments.filter((c) => c.x !== null && c.y !== null && c.version === ad.version);
  const number = new Map(pinned.map((c, i) => [c.id, i + 1]));

  async function send() {
    const text = draft.trim().slice(0, 1200);
    if (!text) {
      setHint("Write a comment first.");
      box.current?.focus();
      return;
    }
    setHint("Saving…");
    if (await onComment(text, pending)) {
      setDraft("");
      setHint(pending ? "Pinned comment added." : "Comment added.");
      setPending(null);
      setPlacing(false);
    } else {
      setHint("");
    }
  }

  return (
    <article className="flex min-w-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono text-[0.7rem] tracking-widest text-muted-foreground uppercase">
          {ad.fmt === "9:16" ? "Story and reel" : `${ad.fmt} feed`} ·{" "}
          {ad.hasCta ? "With CTA" : "No CTA"}
        </span>
        <ReviewState status={ad.status} />
      </div>
      <PhoneFrame
        concept={concept}
        ad={ad}
        page={page}
        zones={zones}
        pins={pinned.map((c) => ({
          id: c.id,
          n: number.get(c.id) ?? 0,
          x: c.x ?? 0,
          y: c.y ?? 0,
          done: c.resolved,
        }))}
        placing={placing}
        pending={pending}
        hot={hot}
        onOpen={onOpen}
        onPlace={(p) => {
          setPending(p);
          setPlacing(false);
          setHint("Pin placed. Say what should change there, then add the comment.");
          box.current?.focus({ preventScroll: true });
        }}
        onPinClick={(id) => {
          setHot(id);
          document.getElementById(`comment-${id}`)?.scrollIntoView({ block: "nearest" });
        }}
      />
      <p className="text-[0.7rem] break-all text-muted-foreground">
        {ad.fileName}
        {ad.version > 1 && ` · version ${ad.version}`}
        {ad.renderCode && ` · render ${ad.renderCode}`}
        {ad.lang === "ar" && " · Arabic, needs native review"}
      </p>
      <div className="flex flex-wrap gap-2">
        <DecisionButton
          active={mine?.status === "approved"}
          tone="ready"
          disabled={busy}
          onClick={() => onDecide("approved")}
        >
          Approve
        </DecisionButton>
        <DecisionButton
          active={mine?.status === "revise"}
          tone="caution"
          disabled={busy}
          onClick={() => {
            onDecide("revise");
            setHint("Add a comment so the team knows what to change.");
            box.current?.focus({ preventScroll: true });
          }}
        >
          Needs changes
        </DecisionButton>
        {mine && (
          <button
            type="button"
            disabled={busy}
            onClick={() => onDecide("clear")}
            className="px-2 text-xs text-muted-foreground hover:text-foreground"
          >
            Clear mine
          </button>
        )}
      </div>
      {votes.length > 0 && (
        <ul className="space-y-1 text-xs text-muted-foreground" aria-label="Decisions so far">
          {votes.map((v) => (
            <li key={v.name} className="flex items-baseline gap-2">
              <span
                aria-hidden="true"
                className={cn(
                  "inline-block size-2",
                  v.status === "revise" ? "bg-status-caution" : "bg-status-ready",
                )}
              />
              <span>
                {REVIEW_LABEL[v.status === "revise" ? "revise" : "approved"]} by{" "}
                <span className="text-foreground">{v.name}</span>, {when(v.createdAt)}
              </span>
            </li>
          ))}
        </ul>
      )}
      <div className="space-y-2 border-t pt-3">
        <div className="flex items-center justify-between gap-2">
          <span className="font-mono text-[0.7rem] tracking-widest uppercase">
            Comments {comments.length > 0 && comments.length}
          </span>
          <button
            type="button"
            aria-pressed={placing || !!pending}
            onClick={() => {
              if (pending) {
                setPending(null);
                setHint("");
              } else {
                setPlacing((p) => !p);
                setHint(placing ? "" : "Now tap the spot on the picture you mean.");
              }
            }}
            className={cn(
              "border px-2.5 py-1 text-xs",
              (placing || pending) && "border-foreground bg-foreground text-background",
            )}
          >
            {pending ? "Remove the pin" : placing ? "Cancel the pin" : "Pin a spot"}
          </button>
        </div>
        {comments.length > 0 && (
          <ol className="space-y-1.5">
            {comments.map((c) => (
              <li
                key={c.id}
                id={`comment-${c.id}`}
                onMouseEnter={() => number.has(c.id) && setHot(c.id)}
                onMouseLeave={() => setHot(null)}
                className={cn(
                  "border px-3 py-2",
                  c.resolved && "opacity-60",
                  hot === c.id && "border-foreground",
                )}
              >
                <p className="text-xs text-muted-foreground">
                  {number.has(c.id) && (
                    <span className="mr-1.5 inline-grid size-4 place-items-center bg-foreground font-mono text-[0.6rem] text-background">
                      {number.get(c.id)}
                    </span>
                  )}
                  <span className="text-foreground">{c.name}</span> · {when(c.createdAt)}
                  {c.version !== ad.version && ` · on version ${c.version}`}
                  {c.resolved && " · done"}
                </p>
                <p className="mt-1 text-sm whitespace-pre-wrap">{c.body}</p>
              </li>
            ))}
          </ol>
        )}
        <label htmlFor={`new-${ad.code}`} className="sr-only">
          Add a comment
        </label>
        <textarea
          id={`new-${ad.code}`}
          ref={box}
          rows={2}
          maxLength={1200}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={pending ? "What should change at this spot?" : "Your comment on this ad"}
          className={textareaClass}
        />
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">{hint}</span>
          <button
            type="button"
            disabled={busy}
            onClick={() => void send()}
            className="shrink-0 bg-foreground px-3 py-1.5 text-xs font-medium text-background disabled:opacity-50"
          >
            Add comment
          </button>
        </div>
      </div>
    </article>
  );
}

function DecisionButton({
  active,
  tone,
  disabled,
  onClick,
  children,
}: {
  active: boolean;
  tone: "ready" | "caution";
  disabled: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "inline-flex flex-1 items-center justify-center gap-2 border px-3 py-2 text-sm font-medium whitespace-nowrap disabled:opacity-50",
        active ? "border-foreground bg-foreground text-background" : "hover:bg-muted",
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "inline-block size-2",
          tone === "ready" ? "bg-status-ready" : "bg-status-caution",
        )}
      />
      {children}
    </button>
  );
}

type FramePin = { id: number; n: number; x: number; y: number; done: boolean };

// A phone mock-up: a feed post for 4:5, 1:1 and 1.91:1, and a full-bleed
// story for 9:16, with the concept's Meta text around the picture.
function PhoneFrame({
  concept,
  ad,
  page,
  zones,
  pins,
  placing,
  pending,
  hot,
  onOpen,
  onPlace,
  onPinClick,
}: {
  concept: BoardConcept;
  ad: BoardAd;
  page: string;
  zones: boolean;
  pins: FramePin[];
  placing: boolean;
  pending: Pin | null;
  hot: number | null;
  onOpen: (src: string, alt: string) => void;
  onPlace: (p: Pin) => void;
  onPinClick: (id: number) => void;
}) {
  const alt = `${concept.code} ${concept.name}, ${ad.fmt}${ad.hasCta ? ", with CTA" : ""}`;
  const dir = ad.lang === "ar" ? "rtl" : undefined;
  const avatar = (
    <span
      aria-hidden="true"
      className="grid size-8 shrink-0 place-items-center bg-neutral-900 text-sm font-semibold text-white"
    >
      {page.charAt(0).toUpperCase()}
    </span>
  );
  const ratio =
    ad.fmt === "9:16"
      ? "9 / 16"
      : ad.fmt === "4:5"
        ? "4 / 5"
        : ad.fmt === "1.91:1"
          ? "1.91 / 1"
          : "1 / 1";
  const picture = (
    <div className="relative" style={{ aspectRatio: ratio }}>
      <button
        type="button"
        aria-label={`Open ${alt} full size`}
        onClick={() => onOpen(fileUrl(ad.file), alt)}
        className="block size-full cursor-zoom-in"
      >
        <img
          src={fileUrl(ad.preview)}
          alt={alt}
          loading="lazy"
          className="size-full object-cover"
        />
      </button>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: placing a pin is a pointer gesture; comments work without one */}
      {/* biome-ignore lint/a11y/useKeyWithClickEvents: as above */}
      <div
        className={cn(
          "absolute inset-0",
          placing ? "cursor-crosshair bg-black/10" : "pointer-events-none",
        )}
        onClick={(e) => {
          if (!placing) return;
          const r = e.currentTarget.getBoundingClientRect();
          const clamp = (v: number) => Math.min(1, Math.max(0, v));
          onPlace({
            x: clamp((e.clientX - r.left) / r.width),
            y: clamp((e.clientY - r.top) / r.height),
          });
        }}
      >
        {pins.map((p) => (
          <button
            key={p.id}
            type="button"
            title={`Comment ${p.n}`}
            onClick={(e) => {
              e.stopPropagation();
              onPinClick(p.id);
            }}
            style={{ left: `${p.x * 100}%`, top: `${p.y * 100}%` }}
            className={cn(
              "pointer-events-auto absolute grid size-5 -translate-x-1/2 -translate-y-1/2 place-items-center border-2 border-white font-mono text-[0.6rem] font-semibold text-white shadow",
              p.done ? "bg-status-ready" : "bg-status-caution",
              hot === p.id && "outline-2 outline-white",
            )}
          >
            {p.n}
          </button>
        ))}
        {pending && (
          <span
            style={{ left: `${pending.x * 100}%`, top: `${pending.y * 100}%` }}
            className="absolute grid size-5 -translate-x-1/2 -translate-y-1/2 place-items-center border-2 border-white bg-foreground text-xs text-background"
          >
            +
          </span>
        )}
      </div>
      {zones && ad.fmt === "9:16" && (
        <>
          <div className="pointer-events-none absolute inset-x-0 top-0 grid h-[14%] place-items-center bg-status-error/50 font-mono text-[0.6rem] tracking-widest text-white uppercase">
            Top 14%: keep clear
          </div>
          <div className="pointer-events-none absolute inset-x-0 bottom-0 grid h-[20%] place-items-center bg-status-error/50 font-mono text-[0.6rem] tracking-widest text-white uppercase">
            Bottom 20%: keep clear
          </div>
        </>
      )}
    </div>
  );

  if (ad.fmt === "9:16") {
    return (
      <div className="mx-auto w-full max-w-72 border-[9px] border-neutral-900 bg-neutral-900">
        <div className="relative">
          {picture}
          {!zones && (
            <>
              <div className="pointer-events-none absolute inset-x-0 top-0 bg-gradient-to-b from-black/50 to-transparent px-3 pt-2.5 pb-6 text-white">
                <div className="mb-2 h-0.5 bg-white/40">
                  <i className="block h-full w-2/5 bg-white" />
                </div>
                <div className="flex items-center gap-2 text-xs">
                  {avatar}
                  <div>
                    <b className="block font-medium">{page}</b>
                    <small className="opacity-80">Sponsored</small>
                  </div>
                </div>
              </div>
              {concept.cta && (
                <div className="pointer-events-none absolute inset-x-0 bottom-0 grid justify-items-center bg-gradient-to-t from-black/50 to-transparent px-3 pt-6 pb-4">
                  <span className="bg-white px-4 py-2 text-xs font-medium text-neutral-900">
                    {concept.cta}
                  </span>
                </div>
              )}
            </>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-72 border-[9px] border-neutral-900 bg-white text-neutral-900">
      <div className="flex items-center gap-2 px-3 pt-3 pb-2 text-xs">
        {avatar}
        <div>
          <b className="block font-medium">{page}</b>
          <small className="text-neutral-500">Sponsored</small>
        </div>
      </div>
      {concept.primaryText && (
        <p dir={dir} className="mx-3 mb-2 line-clamp-3 text-xs">
          {concept.primaryText}
        </p>
      )}
      {picture}
      {(concept.headline || concept.cta) && (
        <div
          dir={dir}
          className="flex items-center justify-between gap-3 border-b border-neutral-200 px-3 py-2.5"
        >
          <div className="min-w-0">
            <b className="block truncate text-xs font-medium">{concept.headline}</b>
            <span className="block truncate text-[0.7rem] text-neutral-500">
              {concept.description}
            </span>
          </div>
          {concept.cta && (
            <span className="shrink-0 bg-neutral-100 px-2.5 py-2 text-[0.7rem] font-medium">
              {concept.cta}
            </span>
          )}
        </div>
      )}
      <div className="h-9" aria-hidden="true" />
    </div>
  );
}

function RendersSheet({
  board,
  pickFor,
  onDone,
  onOpen,
  onPick,
}: {
  board: Board;
  pickFor: string;
  onDone: () => void;
  onOpen: (src: string, alt: string) => void;
  onPick: (concept: string, render: string) => void;
}) {
  const concept = board.concepts.find((c) => c.code === pickFor);
  const inUse = (code: string) =>
    board.concepts
      .filter((c) => c.ads.some((a) => a.renderCode.toUpperCase().startsWith(code)))
      .map((c) => c.code);
  return (
    <section className="py-9">
      <h2 className="text-3xl font-semibold">Renders</h2>
      <p className="mt-2 mb-6 max-w-[70ch] text-sm text-muted-foreground">
        Every render for this project. Choose one for a concept and the team rebuilds its ads on it.
        Tap a render to see it full size.
      </p>
      {concept && (
        <div className="mb-6 flex flex-wrap items-center gap-3 border px-4 py-3 text-sm">
          <span>
            Choosing a render for{" "}
            <b>
              {concept.code} {concept.name}
            </b>
            . Press "Use this render" on the one you want.
          </span>
          <button
            type="button"
            onClick={onDone}
            className="border px-3 py-1.5 text-xs hover:bg-muted"
          >
            Done
          </button>
        </div>
      )}
      {board.renders.length === 0 ? (
        <p className="border border-dashed p-6 text-sm text-muted-foreground">
          No renders have been added yet.
        </p>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,18rem),1fr))] gap-6">
          {board.renders.map((r) => {
            const chosenFor = board.picks.filter((p) => p.render === r.code).map((p) => p.concept);
            const use = inUse(r.code);
            return (
              <article
                key={r.code}
                className={cn(
                  "border",
                  concept && chosenFor.includes(concept.code) && "border-foreground",
                )}
              >
                <button
                  type="button"
                  onClick={() => onOpen(fileUrl(r.file), `Render ${r.code}`)}
                  className="block aspect-[3/2] w-full cursor-zoom-in overflow-hidden bg-muted"
                >
                  <img
                    src={fileUrl(r.preview)}
                    alt={`Render ${r.code}`}
                    loading="lazy"
                    className="size-full object-cover"
                  />
                </button>
                <div className="space-y-1.5 p-3 text-sm">
                  <p className="font-medium">
                    {r.code}
                    {r.label && (
                      <span className="font-normal text-muted-foreground"> · {r.label}</span>
                    )}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {use.length ? `In use now: ${use.join(", ")}` : "Not used yet"}
                  </p>
                  {chosenFor.length > 0 && (
                    <p className="text-xs">Chosen for {chosenFor.join(", ")}</p>
                  )}
                  {concept && (
                    <button
                      type="button"
                      onClick={() => onPick(concept.code, r.code)}
                      className="mt-1 bg-foreground px-3 py-1.5 text-xs font-medium text-background"
                    >
                      Use this render
                    </button>
                  )}
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
