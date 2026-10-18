import { useNavigate, useSearch } from "@tanstack/react-router";
import { type FormEvent, useEffect, useRef, useState } from "react";
import type { Turn } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  useConversations,
  useCreateConversation,
  usePreviewVoice,
  useSendMessage,
  useSendMessageTo,
  useSendVoice,
  useTurns,
} from "@/engine/api/assistant";
import { VoiceInput } from "@/engine/routes/voice-input";

// Ask ADX. The assistant can only read, through tools that run as you.
// Every answer shows the tools it used and the evidence behind them; an
// unavailable model is shown as such, never as an answer.
export function AskPage() {
  const conversations = useConversations();
  const create = useCreateConversation();
  const [current, setCurrent] = useState<number | null>(null);
  const turns = useTurns(current);
  const send = useSendMessage(current);
  const sendTo = useSendMessageTo();
  const sendVoice = useSendVoice(current);
  const preview = usePreviewVoice();
  const [text, setText] = useState("");
  const [spoken, setSpoken] = useState("");
  const { q } = useSearch({ from: "/app/engine/ask" });
  const navigate = useNavigate();
  const asked = useRef<string | null>(null);

  // A question carried in from the desk: one new conversation, the question
  // sent once, the URL cleaned so a reload does not ask it again.
  useEffect(() => {
    if (!q || asked.current === q) return;
    asked.current = q;
    create.mutate(undefined, {
      onSuccess: (c) => {
        setCurrent(c.id);
        sendTo.mutate({ id: c.id, text: q });
        navigate({ to: "/engine/ask", search: {}, replace: true });
      },
    });
  }, [q, create.mutate, sendTo.mutate, navigate]);

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!text.trim() || current === null) return;
    send.mutate(text, { onSuccess: () => setText("") });
  }

  return (
    <section className="grid gap-6 md:grid-cols-[14rem_1fr]">
      <aside className="space-y-2">
        <Button
          size="sm"
          variant="outline"
          disabled={create.isPending}
          onClick={() => create.mutate(undefined, { onSuccess: (c) => setCurrent(c.id) })}
        >
          New conversation
        </Button>
        {conversations.data?.conversations.length === 0 && (
          <p className="text-sm text-muted-foreground">No conversations yet.</p>
        )}
        <ul className="space-y-1 text-sm">
          {conversations.data?.conversations.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                className={`w-full truncate rounded-md px-2 py-1 text-left ${c.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(c.id)}
              >
                {c.title || "Untitled"}
              </button>
            </li>
          ))}
        </ul>
      </aside>

      <div className="space-y-4">
        <h1 className="text-3xl font-semibold tracking-tight">Ask ADX</h1>
        <p className="text-sm text-muted-foreground">
          Read-only. Figures come from ADX’s own calculations through the tools listed under each
          answer; the assistant cannot change budgets, campaigns, leads or settings.
        </p>
        {current === null && <p className="text-muted-foreground">Start or pick a conversation.</p>}
        {current !== null && turns.isPending && <p className="text-muted-foreground">Loading…</p>}
        {turns.error && <p className="text-destructive">{turns.error.message}</p>}
        {turns.data && (
          <ol className="space-y-3">
            {turns.data.turns.map((t) => (
              <TurnView key={t.id} turn={t} />
            ))}
          </ol>
        )}
        {(send.isPending || sendTo.isPending) && (
          <p className="text-sm text-muted-foreground">Asking…</p>
        )}
        {send.error && <p className="text-sm text-destructive">{send.error.message}</p>}
        {sendTo.error && <p className="text-sm text-destructive">{sendTo.error.message}</p>}
        {current !== null && spoken && (
          <div className="space-y-1 rounded-md border p-2 text-sm">
            <p>
              Heard: <span className="font-medium">{spoken}</span>
            </p>
            {preview.data && (
              <p className={preview.data.detected ? "text-destructive" : "text-muted-foreground"}>
                {preview.data.message}
              </p>
            )}
            <div className="flex gap-2">
              <Button
                size="sm"
                disabled={sendVoice.isPending}
                onClick={() => sendVoice.mutate(spoken, { onSuccess: () => setSpoken("") })}
              >
                Send as spoken
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setText(spoken);
                  setSpoken("");
                }}
              >
                Edit first
              </Button>
              <Button size="sm" variant="outline" onClick={() => setSpoken("")}>
                Discard
              </Button>
            </div>
            {sendVoice.data?.preview.detected && (
              <p className="text-destructive">{sendVoice.data.preview.message}</p>
            )}
            {sendVoice.error && <p className="text-destructive">{sendVoice.error.message}</p>}
          </div>
        )}
        {current !== null && (
          <form onSubmit={onSubmit} className="flex gap-2">
            <Input
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="Ask about spend, leads, qualified leads or alerts…"
              aria-label="Your question"
            />
            <Button type="submit" disabled={send.isPending || !text.trim()}>
              Send
            </Button>
            <VoiceInput
              language="en-AE"
              onTranscript={(t) => {
                setSpoken(t);
                preview.mutate(t);
              }}
            />
          </form>
        )}
      </div>
    </section>
  );
}

function TurnView({ turn }: { turn: Turn }) {
  const [open, setOpen] = useState(false);
  if (turn.role === "tool") {
    return (
      <li className="rounded-md border border-dashed p-2 text-xs text-muted-foreground">
        <button
          type="button"
          className="underline underline-offset-2"
          onClick={() => setOpen(!open)}
        >
          Tool {turn.toolName} · {turn.evidenceRefs.length} evidence refs {open ? "▾" : "▸"}
        </button>
        {open && (
          <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap">
            {JSON.stringify({ input: turn.toolInput, result: turn.toolResult }, null, 2)}
          </pre>
        )}
      </li>
    );
  }
  const unavailable = turn.status !== "complete";
  return (
    <li
      className={`rounded-lg p-3 ${turn.role === "user" ? "bg-muted" : unavailable ? "border border-destructive" : "border"}`}
    >
      <div className="mb-1 text-xs text-muted-foreground">
        {turn.role === "user" ? "You" : "ADX"}
        {turn.model ? ` · ${turn.model}` : ""}
        {turn.inputTokens !== null && turn.outputTokens !== null
          ? ` · ${turn.inputTokens}+${turn.outputTokens} tokens`
          : ""}
        {turn.costMicros !== null ? ` · ${(turn.costMicros / 1_000_000).toFixed(4)}` : ""}
        {unavailable ? ` · ${turn.status}` : ""}
      </div>
      {turn.content && <div className="whitespace-pre-wrap text-sm">{turn.content}</div>}
      {turn.toolCalls.length > 0 && (
        <div className="mt-1 text-xs text-muted-foreground">
          Asked for: {turn.toolCalls.map((c) => c.name).join(", ")}
        </div>
      )}
      {turn.warnings.length > 0 && (
        <ul className="mt-1 text-xs text-destructive">
          {turn.warnings.map((w) => (
            <li key={w}>{warningText(w)}</li>
          ))}
        </ul>
      )}
    </li>
  );
}

function warningText(w: string): string {
  if (w.startsWith("action_preview"))
    return `Sounded like a change: ${w.split(": ")[1] ?? ""}. Changes go through proposals; nothing was executed.`;
  if (w.startsWith("ungrounded_numbers"))
    return `Numbers not found in any tool result: ${w.split(": ")[1] ?? ""}. Treat them as unverified.`;
  switch (w) {
    case "no_tool_evidence":
      return "This answer states numbers without consulting any data. Treat them as unverified.";
    case "model_unavailable":
      return "The model was unavailable. Nothing was generated.";
    case "retryable":
      return "You can try again.";
    case "voice_transcript":
      return "Spoken; this is the browser's transcript of what was said.";
    case "model_not_configured":
      return "No model is configured on this server.";
    case "daily_budget_exhausted":
      return "The daily assistant budget is used up.";
    case "tool_round_limit":
      return "Stopped after the maximum number of tool calls.";
    case "answer_truncated":
      return "The answer was cut short by the token limit.";
    case "model_refusal":
      return "The model declined to answer.";
    default:
      return w;
  }
}
