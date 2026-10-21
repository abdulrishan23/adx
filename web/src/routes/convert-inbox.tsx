import { Bot, ChevronDown, MessageCircle, Send, UserRound } from "lucide-react";
import { useState } from "react";
import {
  useCorrectReading,
  useInstructions,
  useSendMessage,
  useThread,
  useThreadAction,
  useThreads,
  useThreadsReadiness,
  useTurnAction,
} from "@/api/conversations";
import type { Thread, ThreadDetail, ThreadSummary, ThreadTurn } from "@/api/types.gen";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Textarea } from "@/components/ui/textarea";
import { formatDateTime, formatRelative, NOT_AVAILABLE } from "@/lib/format";
import { cn } from "@/lib/utils";

type Show = "waiting" | "agent" | "person" | "closed" | "";

const FALLBACK = { chip: "pending" as Status, label: "Unknown" };

const STATE: Record<string, { chip: Status; label: string }> = {
  agent: { chip: "pending", label: "Agent replying" },
  waiting: { chip: "caution", label: "Needs you" },
  person: { chip: "ready", label: "With you" },
  closed: { chip: "pending", label: "Closed" },
};

const HANDOFF: Record<string, string> = {
  requested_human: "Asked for a person",
  objection: "Pushed back, no fact answers it",
  outside_facts: "The agent's reply quoted something not approved",
  question: "Asked something the facts don't carry",
  stop: "Asked to stop",
  no_model: "No model is configured",
};

const TURN: Record<string, { chip: Status; label: string }> = {
  received: { chip: "pending", label: "Received" },
  draft: { chip: "caution", label: "Waiting for you" },
  approved: { chip: "pending", label: "Sending" },
  held: { chip: "caution", label: "Held" },
  sent: { chip: "ready", label: "Sent" },
  delivered: { chip: "ready", label: "Delivered" },
  read: { chip: "ready", label: "Read" },
  failed: { chip: "error", label: "Failed" },
  discarded: { chip: "pending", label: "Discarded" },
};

const OUTCOMES = [
  "interested",
  "not_interested",
  "callback",
  "transfer",
  "no_answer",
  "voicemail",
  "wrong_number",
  "unclear",
] as const;

const OUTCOME_LABEL: Record<(typeof OUTCOMES)[number], string> = {
  interested: "Interested",
  not_interested: "Not interested",
  callback: "Call back",
  transfer: "Hand to a person",
  no_answer: "No answer",
  voicemail: "Voicemail",
  wrong_number: "Wrong number",
  unclear: "Unclear",
};

const ANSWER_LABEL: Record<string, string> = {
  purchase_timeline: "Timeline",
  payment: "Payment",
  purpose: "Purpose",
  unit_type: "Unit type",
  budget: "Budget",
};

// Convert › Inbox: every WhatsApp conversation the agent runs or handed to
// a person, the thread with the agent's drafts and why each one waited,
// and the reading that reached the enquiry. A person can take a thread,
// hand it back, answer in their own words, approve or drop a draft, and
// confirm or correct the reading.
export function ConvertInboxPage() {
  const [show, setShow] = useState<Show>("waiting");
  const [selected, setSelected] = useState<number | null>(null);
  const threads = useThreads(show);
  const readiness = useThreadsReadiness();
  const list = threads.data?.conversations ?? [];
  const current = list.find((t) => t.id === selected) ? selected : (list[0]?.id ?? null);

  const counts = useThreads("");
  const all = counts.data?.conversations ?? [];
  const count = (s: string) => all.filter((t) => t.state === s).length;

  return (
    <>
      <PageHeader
        title="Inbox"
        subtitle={
          <span className="flex flex-col gap-1">
            <span>
              WhatsApp conversations the agent answers from the approved facts, and the ones it
              handed to you.
            </span>
            {readiness.data && readiness.data.reasons.length > 0 && (
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <StatusChip status="caution" className="h-5 px-1.5 text-[0.6rem]">
                  Not sending
                </StatusChip>
                <span>{readiness.data.reasons.join(" · ")}</span>
              </span>
            )}
          </span>
        }
        actions={
          <Segmented
            label="Show"
            value={show}
            onChange={(s) => {
              setShow(s);
              setSelected(null);
            }}
            options={[
              { value: "waiting", label: "Needs you", count: count("waiting"), tone: "caution" },
              { value: "agent", label: "Agent", count: count("agent") },
              { value: "person", label: "With you", count: count("person"), tone: "ready" },
              { value: "closed", label: "Closed", count: count("closed") },
              { value: "", label: "All", count: all.length },
            ]}
          />
        }
      />
      {threads.error && <ErrorNote error={threads.error} />}
      {threads.isPending && <LoadingRows rows={6} />}
      {threads.data && list.length === 0 && (
        <Panel>
          <EmptyState title="Nothing here" icon={<MessageCircle />}>
            {show === "waiting"
              ? "No conversation is waiting for a person."
              : "No conversation in this state."}
          </EmptyState>
        </Panel>
      )}
      {list.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-[22rem_1fr]">
          <ThreadList list={list} selected={current} onSelect={setSelected} />
          {current !== null && <ThreadView id={current} />}
        </div>
      )}
    </>
  );
}

function ThreadList({
  list,
  selected,
  onSelect,
}: {
  list: ThreadSummary[];
  selected: number | null;
  onSelect: (id: number) => void;
}) {
  return (
    <ul className="divide-y rounded-lg border bg-card">
      {list.map((t) => {
        const st = STATE[t.state] ?? FALLBACK;
        return (
          <li key={t.id}>
            <button
              type="button"
              aria-current={t.id === selected}
              onClick={() => onSelect(t.id)}
              className={cn(
                "flex w-full flex-col gap-1 px-4 py-3 text-left hover:bg-muted/50",
                t.id === selected && "bg-muted",
              )}
            >
              <span className="flex items-center justify-between gap-2">
                <span className="truncate font-medium">{t.personName || t.address}</span>
                <span className="shrink-0 font-mono text-xs text-muted-foreground">
                  {formatRelative(t.updatedAt)}
                </span>
              </span>
              <span className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                <StatusMark status={st.chip} />
                <span className="shrink-0">{st.label}</span>
                {t.handoffKind && (
                  <span className="truncate">· {HANDOFF[t.handoffKind] ?? t.handoffKind}</span>
                )}
                {t.pending > 0 && (
                  <span className="shrink-0">
                    · {t.pending} draft{t.pending === 1 ? "" : "s"}
                  </span>
                )}
              </span>
              <span className="truncate text-sm text-muted-foreground">
                {t.projectName ? `${t.projectName} · ` : ""}
                {t.lastBody || "No message yet"}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

function ThreadView({ id }: { id: number }) {
  const thread = useThread(id);
  if (thread.isPending) return <LoadingRows rows={8} />;
  if (thread.error) return <ErrorNote error={thread.error} />;
  if (!thread.data) return null;
  return <ThreadBody detail={thread.data} />;
}

function ThreadBody({ detail }: { detail: ThreadDetail }) {
  const c = detail.conversation;
  const act = useThreadAction();
  const send = useSendMessage();
  const [body, setBody] = useState("");
  const [note, setNote] = useState("");
  const st = STATE[c.state] ?? FALLBACK;
  const busy = act.isPending || send.isPending;
  const error = act.error ?? send.error;
  const windowOpen = c.windowClosesAt ? Date.parse(c.windowClosesAt) > Date.now() : false;

  // The composer empties when the thread changes, not when it refreshes.
  const [shown, setShown] = useState(c.id);
  if (shown !== c.id) {
    setShown(c.id);
    setBody("");
    setNote("");
  }

  return (
    <div className="space-y-4">
      <Panel
        title={
          <span className="flex flex-wrap items-center gap-3">
            {detail.person.displayName || c.address}
            <StatusChip status={st.chip}>{st.label}</StatusChip>
          </span>
        }
        description={
          <span className="font-mono text-xs">
            {c.address}
            {detail.projectName ? ` · ${detail.projectName}` : " · no project yet"}
            {c.windowClosesAt &&
              (windowOpen
                ? ` · free replies until ${formatDateTime(c.windowClosesAt)}`
                : " · window closed, a template is needed")}
          </span>
        }
        action={
          <div className="flex flex-wrap items-center gap-1.5">
            {c.state !== "person" && c.state !== "closed" && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => act.mutate({ id: c.id, action: "take", note })}
              >
                Take over
              </Button>
            )}
            {c.state !== "agent" && c.state !== "closed" && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => act.mutate({ id: c.id, action: "hand-back" })}
              >
                Hand back to the agent
              </Button>
            )}
            {c.state !== "closed" && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => act.mutate({ id: c.id, action: "close", note })}
              >
                Close
              </Button>
            )}
          </div>
        }
        bodyClassName="p-0"
      >
        {c.handoffKind && c.state !== "closed" && (
          <div className="flex gap-2.5 border-b bg-status-caution/15 px-5 py-3 text-sm">
            <UserRound className="mt-0.5 size-4 shrink-0 text-status-caution" aria-hidden="true" />
            <p>
              <span className="font-medium">{HANDOFF[c.handoffKind] ?? c.handoffKind}. </span>
              {c.handoffKind === "requested_human"
                ? `“${c.stateReason.split(": ").slice(1).join(": ")}”`
                : c.stateReason}
            </p>
          </div>
        )}
        <ol className="space-y-3 px-5 py-4">
          {detail.turns.map((t) => (
            <TurnRow key={t.id} turn={t} />
          ))}
        </ol>
        <form
          className="space-y-2 border-t px-5 py-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!body.trim()) return;
            send.mutate({ id: c.id, body }, { onSuccess: () => setBody("") });
          }}
        >
          <Textarea
            aria-label="Your message"
            rows={2}
            placeholder={
              c.state === "closed"
                ? "This conversation is closed."
                : windowOpen
                  ? "Write as yourself. Sending takes the thread from the agent."
                  : "The window has closed; a free reply is held until the customer writes again."
            }
            disabled={c.state === "closed" || busy}
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Input
              aria-label="Note"
              className="h-8 max-w-xs"
              placeholder="Note for take over or close"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            <Button type="submit" size="sm" disabled={c.state === "closed" || busy || !body.trim()}>
              <Send /> Send
            </Button>
          </div>
          {error && <ErrorNote error={error} />}
        </form>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-2">
        <ReadingPanel c={c} />
        <InstructionsPanel id={c.id} />
      </div>
    </div>
  );
}

function TurnRow({ turn: t }: { turn: ThreadTurn }) {
  const turnAct = useTurnAction();
  const out = t.direction === "out";
  const agent = t.author === "agent";
  const waiting = t.state === "draft" || t.state === "held" || t.state === "failed";
  const unapprovable = t.handoff === "outside_facts";
  const st = TURN[t.state] ?? FALLBACK;
  return (
    <li className={cn("flex", out ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[85%] space-y-1.5 rounded-lg px-3.5 py-2.5 text-sm",
          out ? "bg-foreground text-background" : "bg-muted",
          t.state === "discarded" && "opacity-50",
        )}
      >
        <p
          className={cn(
            "flex items-center gap-1.5 font-mono text-[0.65rem] tracking-wider uppercase",
            out ? "text-background/60" : "text-muted-foreground",
          )}
        >
          {agent ? <Bot className="size-3" /> : out ? <UserRound className="size-3" /> : null}
          {agent ? "Adx agent" : out ? "Reportage" : "Customer"}
          <span>· {formatDateTime(t.occurredAt)}</span>
          {out && (
            <span className="inline-flex items-center gap-1">
              · <StatusMark status={st.chip} className={out ? "border-background" : ""} />{" "}
              {st.label}
            </span>
          )}
        </p>
        <p className="whitespace-pre-wrap">{t.body}</p>
        {t.reason && (
          <p className={cn("text-xs", out ? "text-background/70" : "text-muted-foreground")}>
            {t.reason}
          </p>
        )}
        {waiting && (
          <div className="flex flex-wrap gap-1.5 pt-1">
            {!unapprovable && (
              <Button
                size="xs"
                variant="secondary"
                disabled={turnAct.isPending}
                onClick={() => turnAct.mutate({ id: t.id, action: "approve" })}
              >
                {t.state === "failed" ? "Try again" : "Approve and send"}
              </Button>
            )}
            <Button
              size="xs"
              variant="secondary"
              disabled={turnAct.isPending}
              onClick={() =>
                turnAct.mutate({ id: t.id, action: "discard", note: "dropped from the inbox" })
              }
            >
              Discard
            </Button>
          </div>
        )}
        {turnAct.error && <p className="text-xs">{turnAct.error.message}</p>}
      </div>
    </li>
  );
}

function ReadingPanel({ c }: { c: Thread }) {
  const act = useThreadAction();
  const correct = useCorrectReading();
  const [outcome, setOutcome] = useState<(typeof OUTCOMES)[number] | null>(null);
  const [why, setWhy] = useState("");
  const answers = Object.entries(c.answers ?? {});
  const review: Record<string, { chip: Status; label: string }> = {
    "": { chip: "pending", label: "Not read yet" },
    auto: { chip: "ready", label: "Applied" },
    pending: { chip: "caution", label: "Waiting for you" },
    confirmed: { chip: "ready", label: "Confirmed" },
    corrected: { chip: "ready", label: "Corrected" },
  };
  const rv = review[c.reviewStatus] ?? { chip: "pending" as Status, label: "Not read yet" };
  return (
    <Panel
      title="What Adx learned"
      description={
        c.readAt
          ? `Read ${formatRelative(c.readAt)} over ${c.readTurns} message${c.readTurns === 1 ? "" : "s"}`
          : "The thread is read at a handoff, when the agent is done, or when you close it."
      }
      action={<StatusChip status={rv.chip}>{rv.label}</StatusChip>}
    >
      <dl className="space-y-2.5 text-sm">
        <Field label="Outcome">
          {c.outcome
            ? (OUTCOME_LABEL[c.outcome as (typeof OUTCOMES)[number]] ?? c.outcome)
            : NOT_AVAILABLE}
          {c.confidence && (
            <span className="ml-2 font-mono text-[0.65rem] tracking-wider text-muted-foreground uppercase">
              {c.confidence === "high" ? "Sure" : "Unsure"}
            </span>
          )}
        </Field>
        {c.summary && <Field label="Summary">{c.summary}</Field>}
        {answers.map(([k, a]) => (
          <Field key={k} label={ANSWER_LABEL[k] ?? k}>
            {a.value || NOT_AVAILABLE}
            {a.uncertain && (
              <StatusChip status="caution" className="ml-2 h-5 px-1.5 text-[0.6rem]">
                Uncertain, not recorded
              </StatusChip>
            )}
            {a.quote && <span className="block text-xs text-muted-foreground">“{a.quote}”</span>}
          </Field>
        ))}
        {c.applied.stage && <Field label="Stage set">{c.applied.stage}</Field>}
        {c.applied.held.length > 0 && (
          <Field label="Held">
            <ul className="space-y-1 text-xs text-muted-foreground">
              {c.applied.held.map((h) => (
                <li key={h}>{h}</li>
              ))}
            </ul>
          </Field>
        )}
        {c.reviewReason && (
          <Field label="Note">
            <span className="text-muted-foreground">{c.reviewReason}</span>
          </Field>
        )}
      </dl>
      <div className="mt-4 flex flex-wrap items-center gap-2 border-t pt-4">
        {c.reviewStatus === "pending" && c.outcome && (
          <Button
            size="sm"
            disabled={act.isPending}
            onClick={() => act.mutate({ id: c.id, action: "confirm" })}
          >
            Confirm reading
          </Button>
        )}
        <Button
          size="sm"
          variant="outline"
          disabled={act.isPending}
          onClick={() => act.mutate({ id: c.id, action: "read" })}
        >
          Read now
        </Button>
      </div>
      <form
        className="mt-4 space-y-2 border-t pt-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!outcome || !why.trim()) return;
          correct.mutate(
            { id: c.id, input: { outcome, answers: {}, note: why } },
            { onSuccess: () => setWhy("") },
          );
        }}
      >
        <Eyebrow>Correct the reading</Eyebrow>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            aria-label="Outcome"
            value={outcome}
            onValueChange={setOutcome}
            placeholder="Outcome"
            className="w-44"
            options={OUTCOMES.map((o) => ({ value: o, label: OUTCOME_LABEL[o] }))}
          />
          <Input
            aria-label="Why the reading was wrong"
            className="h-8 min-w-48 flex-1"
            placeholder="Why it was wrong"
            value={why}
            onChange={(e) => setWhy(e.target.value)}
          />
          <Button
            type="submit"
            size="sm"
            variant="outline"
            disabled={correct.isPending || !outcome || !why.trim()}
          >
            Correct
          </Button>
        </div>
        {correct.error && <ErrorNote error={correct.error} />}
      </form>
    </Panel>
  );
}

function InstructionsPanel({ id }: { id: number }) {
  const [open, setOpen] = useState(false);
  const instructions = useInstructions(open ? id : null);
  return (
    <Panel
      title="What the agent was told"
      description="The brief the agent answers from: the approved facts, the questions and the rules."
      action={
        <Button size="sm" variant="ghost" onClick={() => setOpen((o) => !o)}>
          <ChevronDown className={cn("transition-transform", open && "rotate-180")} />
          {open ? "Hide" : "Show"}
        </Button>
      }
    >
      {!open && <p className="text-sm text-muted-foreground">Hidden.</p>}
      {open && instructions.isPending && <LoadingRows rows={4} />}
      {open && instructions.error && <ErrorNote error={instructions.error} />}
      {open && instructions.data && (
        <pre className="font-mono text-xs leading-relaxed whitespace-pre-wrap text-muted-foreground">
          {instructions.data.instructions}
        </pre>
      )}
    </Panel>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[7.5rem_1fr] gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0">{children}</dd>
    </div>
  );
}
