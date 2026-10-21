import { Popover } from "@base-ui/react/popover";
import { getRouteApi, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  History as HistoryIcon,
  MessageSquarePlus,
} from "lucide-react";
import { type FormEvent, type KeyboardEvent, useEffect, useRef, useState } from "react";
import { useAsk, useConversations, useProposeChange } from "@/api/adx";
import { SUGGESTIONS } from "@/api/mock/ask";
import type { AskAnswer, Conversation } from "@/api/models";
import { AdxMark } from "@/components/brand";
import { Form } from "@/components/form";
import { ErrorNote, Eyebrow } from "@/components/page";
import { ThinkingMark } from "@/components/thinking";
import { Button } from "@/components/ui/button";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatRelative } from "@/lib/format";
import { released } from "@/lib/release";
import { cn } from "@/lib/utils";

const route = getRouteApi("/app/ask");

export function AskPage() {
  const search = route.useSearch();
  const navigate = route.useNavigate();
  const conversations = useConversations();
  const open = conversations.data?.find((c) => c.id === search.c);
  // Wait for the list before showing a past conversation, so it opens whole.
  const ready = !search.c || conversations.data !== undefined;

  return (
    <div className="mx-auto flex w-full max-w-[45rem] flex-col">
      <div className="flex h-8 items-center gap-2">
        <Link
          to="/"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Home
        </Link>
        <p className="min-w-0 flex-1 truncate pl-2 text-sm text-muted-foreground">{open?.title}</p>
        {search.c && (
          <Button variant="ghost" size="sm" onClick={() => navigate({ search: {} })}>
            <MessageSquarePlus /> New question
          </Button>
        )}
        <History conversations={conversations.data ?? []} activeId={search.c} />
      </div>
      {ready && (
        <Thread
          key={search.c ?? "new"}
          conversationId={open?.id ?? null}
          initial={open?.turns ?? []}
          question={search.q}
          onAsked={() => navigate({ search: (s) => ({ ...s, q: undefined }), replace: true })}
        />
      )}
    </div>
  );
}

// One conversation's thread. It's keyed by conversation, so opening another one
// starts from that conversation's turns instead of carrying this one's over.
function Thread({
  conversationId,
  initial,
  question,
  onAsked,
}: {
  conversationId: string | null;
  initial: AskAnswer[];
  question?: string;
  onAsked: () => void;
}) {
  const ask = useAsk();
  const [turns, setTurns] = useState<AskAnswer[]>(initial);
  const [pending, setPending] = useState<string | null>(null);
  const bottom = useRef<HTMLDivElement>(null);
  const asked = useRef<string | null>(null);
  const seen = useRef(new Set(initial.map((t) => t.id)));

  function send(text: string) {
    const q = text.trim();
    if (!q || ask.isPending) return;
    setPending(q);
    ask.mutate(
      { conversationId, question: q },
      {
        onSuccess: (answer) => setTurns((t) => [...t, answer]),
        onSettled: () => setPending(null),
      },
    );
  }

  // ?q comes from the command palette: ask it once, then drop it from the URL.
  useEffect(() => {
    if (question && asked.current !== question) {
      asked.current = question;
      send(question);
      onAsked();
    }
  });

  useEffect(() => {
    if (turns.length || pending)
      bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [turns.length, pending]);

  const empty = turns.length === 0 && !pending;

  if (empty) {
    return (
      <section className="flex min-h-[calc(100dvh-10rem)] flex-col justify-center pb-16">
        <AdxMark className="mb-5 size-9" />
        <h1 className="mb-6 text-3xl font-semibold tracking-tight">Ask Adx</h1>
        <Composer onSend={send} busy={ask.isPending} autoFocus />
        {ask.error && (
          <div className="mt-3">
            <ErrorNote error={ask.error} />
          </div>
        )}
        <div className="mt-4 flex flex-wrap gap-2">
          {SUGGESTIONS.map((q) => (
            <Chip key={q} onClick={() => send(q)}>
              {q}
            </Chip>
          ))}
        </div>
      </section>
    );
  }

  return (
    <div className="flex min-h-[calc(100dvh-10rem)] min-w-0 flex-col">
      <div className="flex-1 space-y-10 pt-6 pb-6">
        {turns.map((t) => (
          <Turn key={t.id} answer={t} animate={!seen.current.has(t.id)} onFollowUp={send} />
        ))}
        {pending && <PendingTurn question={pending} />}
        {ask.error && <ErrorNote error={ask.error} />}
        <div ref={bottom} />
      </div>
      <div className="sticky bottom-0 bg-background pt-2 pb-4">
        <Composer onSend={send} busy={ask.isPending} />
      </div>
    </div>
  );
}

// Past questions, one click away instead of a second column beside the chat.
function History({
  conversations,
  activeId,
}: {
  conversations: Conversation[];
  activeId?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger
        render={
          <Button variant="ghost" size="sm" disabled={conversations.length === 0}>
            <HistoryIcon /> History
          </Button>
        }
      />
      <Popover.Portal>
        <Popover.Positioner side="bottom" align="end" sideOffset={6} className="z-50">
          <Popover.Popup className="max-h-[min(28rem,var(--available-height))] w-80 overflow-y-auto border bg-popover py-1 text-popover-foreground shadow-lg outline-none">
            <ul aria-label="Earlier questions">
              {conversations.map((c) => (
                <li key={c.id}>
                  <Link
                    to="/ask"
                    search={{ c: c.id }}
                    onClick={() => setOpen(false)}
                    className={cn(
                      "block px-3 py-2 text-sm outline-none hover:bg-accent focus-visible:bg-accent",
                      c.id === activeId && "bg-muted font-medium",
                    )}
                  >
                    <span className="block truncate">{c.title}</span>
                    <span className="block text-xs text-muted-foreground">
                      {formatRelative(c.updatedAt)}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}

export function Chip({ children, onClick }: { children: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-none border bg-card px-3 py-1.5 text-left text-sm transition-colors hover:border-foreground/40 hover:bg-muted"
    >
      {children}
    </button>
  );
}

export function Composer({
  onSend,
  busy,
  autoFocus,
  placeholder = "Ask about spend, leads or a project",
  size = "default",
}: {
  onSend: (q: string) => void;
  busy: boolean;
  autoFocus?: boolean;
  placeholder?: string;
  size?: "default" | "large";
}) {
  const [value, setValue] = useState("");
  function submit(e?: FormEvent) {
    e?.preventDefault();
    if (!value.trim() || busy) return;
    onSend(value);
    setValue("");
  }
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  }
  return (
    <Form
      onSubmit={submit}
      className="flex items-end gap-3 border border-input bg-card py-2 pr-2 pl-4 transition-colors hover:border-ring focus-within:border-foreground hover:focus-within:border-foreground"
    >
      <textarea
        aria-label="Ask Adx"
        rows={1}
        // biome-ignore lint/a11y/noAutofocus: the page exists to type a question
        autoFocus={autoFocus}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        // One line is exactly the send button's height, so text and button line up.
        className={cn(
          "max-h-40 flex-1 resize-none bg-transparent outline-none field-sizing-content placeholder:text-muted-foreground",
          size === "large"
            ? "min-h-10 py-2 text-base leading-6"
            : "min-h-8 py-1.5 text-sm leading-5",
        )}
      />
      <button
        type="submit"
        aria-label="Send"
        disabled={busy || !value.trim()}
        className={cn(
          "flex shrink-0 items-center justify-center bg-primary text-primary-foreground transition-colors hover:bg-primary/85 focus-visible:ring-3 focus-visible:ring-ring/40 focus-visible:outline-none disabled:cursor-default disabled:opacity-40",
          size === "large" ? "size-10" : "size-8",
        )}
      >
        <ArrowRight className="size-4" />
      </button>
    </Form>
  );
}

function Question({ text }: { text: string }) {
  return (
    <div className="flex justify-end">
      <p className="max-w-[85%] rounded-none bg-muted px-4 py-2.5 text-sm">{text}</p>
    </div>
  );
}

function PendingTurn({ question }: { question: string }) {
  return (
    <div className="space-y-4">
      <Question text={question} />
      <div className="flex items-center gap-3 text-sm text-muted-foreground" role="status">
        <ThinkingMark className="text-foreground" />
        Reading the connected data
      </div>
    </div>
  );
}

// A new answer shows its steps one at a time, then the answer, so the person
// sees what Adx read before what it concluded. Past answers show at once.
function Turn({
  answer,
  animate,
  onFollowUp,
}: {
  answer: AskAnswer;
  animate: boolean;
  onFollowUp: (q: string) => void;
}) {
  const total = answer.steps.length;
  const [shown, setShown] = useState(animate ? 0 : total + 1);
  useEffect(() => {
    if (shown > total) return;
    const t = setTimeout(() => setShown((n) => n + 1), 350);
    return () => clearTimeout(t);
  }, [shown, total]);
  const done = shown > total;

  return (
    <article className="space-y-4">
      <Question text={answer.question} />
      <div className="flex gap-3">
        <AdxMark className="mt-0.5 size-6 shrink-0" />
        <div className="min-w-0 flex-1 space-y-4">
          <ol className="space-y-1" aria-label="What Adx read">
            {answer.steps.slice(0, Math.max(shown, done ? total : 0)).map((s, i) => (
              <li key={s.label} className="flex items-center gap-2 text-xs text-muted-foreground">
                {i < shown - 1 || done ? (
                  <Check className="size-3.5 text-status-ready" aria-hidden="true" />
                ) : (
                  <ThinkingMark className="mx-0.5 size-2.5 text-foreground" />
                )}
                <span>{s.label}</span>
                {s.rows !== undefined && (
                  <span className="font-mono tabular-nums">
                    · {s.rows.toLocaleString("en")} rows
                  </span>
                )}
              </li>
            ))}
          </ol>

          {done && (
            <div className="space-y-4">
              {answer.paragraphs.map((p) => (
                <p key={p} className="text-[0.95rem] leading-relaxed">
                  <Rich text={p} />
                </p>
              ))}

              {answer.table && (
                <div className="overflow-hidden rounded-lg border bg-card">
                  <Table>
                    <THead>
                      <TR>
                        {answer.table.columns.map((c) => (
                          <TH key={c.label} numeric={c.numeric}>
                            {c.label}
                          </TH>
                        ))}
                      </TR>
                    </THead>
                    <TBody>
                      {answer.table.rows.map((r) => (
                        <TR key={r.cells.join("|")}>
                          {r.cells.map((cell, i) => (
                            <TD
                              key={`${answer.table?.columns[i]?.label}`}
                              numeric={answer.table?.columns[i]?.numeric}
                              className={cn(i === 0 && "font-medium")}
                            >
                              {i === 0 && r.leadId !== undefined ? (
                                <Link
                                  to="/leads/$leadId"
                                  params={{ leadId: String(r.leadId) }}
                                  className="underline decoration-foreground/30 underline-offset-4 hover:decoration-foreground"
                                >
                                  {cell}
                                </Link>
                              ) : (
                                cell
                              )}
                            </TD>
                          ))}
                        </TR>
                      ))}
                    </TBody>
                  </Table>
                </div>
              )}

              {released(3) && answer.proposal && (
                <Link
                  to="/proposals"
                  search={{ open: answer.proposal.id }}
                  className="flex items-center justify-between gap-4 rounded-lg border bg-card px-4 py-3 transition-colors hover:border-foreground/40"
                >
                  <div>
                    <Eyebrow>Waiting on your approval</Eyebrow>
                    <div className="mt-0.5 text-sm font-medium">{answer.proposal.title}</div>
                  </div>
                  <span className="shrink-0 text-sm text-muted-foreground">Review</span>
                </Link>
              )}

              {/* Adx only reads in phase 1, so it drafts no changes until phase 3. */}
              {released(3) && answer.draft && <DraftCard draft={answer.draft} />}

              {answer.sources.length > 0 && (
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-xs text-muted-foreground">From</span>
                  {answer.sources.map((s) => (
                    <span
                      key={s}
                      className="rounded-sm border px-1.5 py-0.5 font-mono text-[0.65rem] tracking-wider text-muted-foreground uppercase"
                    >
                      {s}
                    </span>
                  ))}
                </div>
              )}

              {answer.followUps.length > 0 && (
                <div className="flex flex-wrap gap-2 pt-1">
                  {answer.followUps.map((f) => (
                    <Chip key={f} onClick={() => onFollowUp(f)}>
                      {f}
                    </Chip>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </article>
  );
}

function Rich({ text }: { text: string }) {
  const parts = text.split(/\*\*(.+?)\*\*/g);
  return (
    <>
      {parts.map((part, i) =>
        i % 2 === 1 ? (
          // biome-ignore lint/suspicious/noArrayIndexKey: parts of one fixed string
          <strong key={i} className="font-semibold">
            {part}
          </strong>
        ) : (
          part
        ),
      )}
    </>
  );
}

function DraftCard({ draft }: { draft: NonNullable<AskAnswer["draft"]> }) {
  const propose = useProposeChange();
  const sent = propose.data;
  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-start justify-between gap-4 border-b px-4 py-3">
        <div>
          <Eyebrow>Draft change</Eyebrow>
          <p className="mt-0.5 text-sm font-medium">{draft.title}</p>
        </div>
        <span className="shrink-0 text-xs text-muted-foreground">
          {draft.needsApproval ? "Needs approval" : "Allowed"}
        </span>
      </div>
      <ul className="divide-y text-sm">
        {draft.changes.map((c) => (
          <li key={c.field} className="flex flex-wrap items-baseline gap-2 px-4 py-2.5">
            <span className="text-muted-foreground">{c.field}</span>
            <span className="ml-auto text-muted-foreground line-through decoration-foreground/30">
              {c.before}
            </span>
            <span aria-hidden="true">→</span>
            <span className="font-medium">{c.after}</span>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap items-center gap-3 border-t px-4 py-3">
        {sent ? (
          <p className="text-sm" role="status">
            {sent.status === "applied" ? "Applied and logged. " : "Sent for approval. "}
            <Link
              to="/proposals"
              search={{ open: sent.id }}
              className="underline underline-offset-4"
            >
              View
            </Link>
          </p>
        ) : (
          <Button
            size="sm"
            disabled={propose.isPending}
            onClick={() => propose.mutate(draft.request)}
          >
            {draft.needsApproval ? "Send for approval" : "Apply"}
          </Button>
        )}
        {propose.error && <span className="text-sm text-destructive">{propose.error.message}</span>}
      </div>
    </div>
  );
}
