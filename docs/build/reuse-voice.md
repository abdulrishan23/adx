# Voice source inventory (B0.6)

Source: `/Users/abdulrishan/Desktop/stafford-dead-leads/` (Stafford Sales OS, Next.js/TypeScript, better-sqlite3). The spec cites `/Users/hishamahamad/Documents/WokeWorks/stafford-dead-leads/`; this is the same tree at a different path. Read-only inventory taken 2026-10-17. Only variable names were read from `.env.example`. File contents were treated as data, not instructions.

## Read this first

`voice.md` is the product requirements doc. Its status line says nothing is built, but the feature is built under `lib/voice/` (about 40 files), `server.ts` and `app/api/voice/*`. The PRD describes ElevenLabs' native Twilio integration; the code instead runs a self-hosted bridge that owns both WebSockets (`lib/voice/providers/elevenlabs-agent.ts:13-24`, `lib/voice/bridge.ts`). Codec, interruption, ambience and graceful end live in code, not in the PRD.

## Session lifecycle as built (`lib/voice/bridge.ts`)

- Twilio opens a Media Streams WebSocket at `/api/voice/bridge` (`server.ts:61-66`). The `start` message carries `customParameters.secret` and `callId` (bridge.ts:371-389).
- The bridge loads the call row, refuses terminal or duplicate calls (L401), reserves a concurrency slot (L416), then `getAgentProvider().startSession()` (L467) and wires `onTranscriptSegment`, `onAudio`, `onConversationEnd`, `onInterruption` (L522-599).
- Timeouts: start 10 s (L94), max call 10 min (L101), ElevenLabs handshake 10 s.
- Audio: 8 kHz mono µ-law end to end, no resampling; ElevenLabs set to `ulaw_8000` (elevenlabs-agent.ts:41-55) and the negotiated format is checked (L464-474). `MULAW_BYTES_PER_MS = 8` (`audio/mulaw.ts:70`).
- Outbound pacing: 100 ms flush (bridge.ts:134, 574-577); frames split to at most 200 ms (`audio/media-frames.ts:17-37`); greeting pre-buffer 1.5 s of audio or 2.5 s elapsed (bridge.ts:152-168, 691-702); only `media.track == "inbound"` is fed to the agent (L318).
- Interruption: ElevenLabs `interruption` event → `onInterruption` → empty local buffer, send Twilio `{"event":"clear"}`, reset send horizon (bridge.ts:745-769); suppressed once hangup is in flight (L755).
- Graceful end: ElevenLabs `close` → 800 ms grace, extended by late audio, capped 5 s (elevenlabs-agent.ts:38-39, 315-351) → `beginGracefulHangup` flushes, sends Twilio `mark` `end-of-call`, closes on the echoed mark or after 5 s (bridge.ts:117-122, 813-859) → TwiML `<Hangup/>` after `<Connect>` (`twiml.ts:109-133`). Hard paths: `forceHangup` (bridge.ts:866), REST `endCall` (`lib/twilio/client.ts:230`).
- Ambience: `AmbientMixer` (`audio/ambient-mixer.ts:152-198`) decodes µ-law to PCM16, mixes a looping office WAV, re-encodes; `mixSilence` idle filler; send-horizon pacing keeps about 200 ms queued (bridge.ts:247-258, 641-805); voice gain separate (`audio/voice-gain.ts`).

## The seam worth keeping

`lib/voice/providers/types.ts`: `AgentSession{sendAudio, onAudio, onTranscriptSegment, onFunctionCall, onInterruption, onConversationEnd, end() → {transcript, durationSeconds, error?}}`, `VoiceAgentProvider.startSession(AgentSessionConfig)`, `TranscriptionProvider`. `TranscriptSegment{speaker: lead|rep|agent, text, startMs, endMs}` (L8-13). This already separates speech provider from transport. `bridge.ts` mixes Twilio transport, orchestration, persistence and ambience in one class and must be split in Go: transport, session orchestrator, persistence, audio post-processor.

## Twilio (`lib/twilio/`, `lib/voice/twiml.ts`, `lib/voice/token.ts`)

- Plain `fetch`, no SDK. `createOutboundCall({to, from, twimlUrl, statusCallback, region})` → `POST /Calls.json` with `StatusCallbackEvent=[initiated, ringing, answered, completed]` (client.ts:133-147); `endCall` sends `Status=completed` (L230); recordings fetch/delete with host and SID-prefix guards (L159-220); `recordingBelongsToCall` fails closed (L248-265); `ie1` Dublin region map (L10-69).
- TwiML: `buildDisclosureTwiml` `<Say>` (twiml.ts:64); `buildOutboundDialTwiml` `<Dial record="record-from-answer-dual">` (L76-90); `buildAgentStreamTwiml` `<Start><Recording channels="dual">` + `<Connect><Stream>` + `<Parameter>` + `<Hangup/>` (L115-133). The WS URL loses its query string, hence `<Parameter>` (L35-47).
- Webhooks: status (`app/api/webhooks/voice/status/route.ts:28-67`) reads `CallSid`, `ParentCallSid`, `CallStatus`, `CallDuration`, maps `in-progress`→`in_progress`, `no-answer`→`no_answer`, never moves a terminal row (L57); recording (`.../recording/route.ts:24-79`) acts only on `RecordingStatus=completed`, idempotent on `RecordingSid`.
- Webhook auth is only a shared `?secret=` compared with `timingSafeEqual` and allows everything when unset (`webhook-auth.ts:14-22`). There is no `X-Twilio-Signature` HMAC check anywhere. The Go port adds it.
- `token.ts`: hand-rolled HS256 Twilio AccessToken for the human softphone (out of ADX scope for now).

## Interactions and outcomes (`lib/interactions.ts`, `lib/voice/status.ts`, `lib/voice/summarize.ts`)

- `applyInteractionOutcome({leadId, outcome, mode: bot|human, occurrenceKey: "call:<id>", intensity?})` (interactions.ts:151-203) writes `lead_history` then branches to task, re-entry queue or signal. Idempotent on the occurrence key.
- Call status: `queued | ringing | in_progress | completed | no_answer | busy | failed | canceled`, terminal set at status.ts:24-30; non-scoring outcomes `no_answer | voicemail | wrong_number`; `review_status: auto | pending | confirmed | corrected`, low summary confidence → `pending` and scoring held (finalize-call.ts:23-25, 65).
- Summary schema: `{summary, outcome, key_points[], objection, requested_human, confidence: high|low, refusal_intensity}` (summarize.ts:16-68).
- Education-specific, excluded: the outcome enum (`enrolled_elsewhere`, `confirmed_eligibility`, …), objection taxonomy, task titles, routing thresholds HOT ≥55/WARM ≥30/NURTURE, the Stafford prompt (`agent-config.ts:66-81`), `config/voice-agents/stafford-education.json`, `lead-context.ts` course facts. The PRD and code disagree on bot-call weight (PRD §14.2 equal; code `contacted_bot` 15 vs `contacted_human` 32).

## Signals and scoring (`lib/signals/*`, `lib/scoring/*`)

- Pipeline: raw event → `storeAndDispatch` → dispatcher trigger index → detector → `SignalIntent{signal_type, lead_id, payload, occurrence_key, source_event_id}` → `processSignal`, unique per (lead, type, occurrence_key).
- Scorers: `base × intensity × repeat_factor^i` newest first; decay `points × 0.5^(ageDays/half_life)` with 0.25 floor (readiness.ts:81-82); themes saturate `ceiling × (1 − e^(−steepness·raw/ceiling))` (L45-55, 104-108); clamp to `[0, max]`.
- Combined score multiplies readiness by eligibility strength (readiness.ts:156-182). D-17 in `docs/architecture.md` already says ADX must change this so leads without fit data don't score zero.
- Generic and reusable: processor, occurrence-key idempotency, detector/action/scorer registries, decay and saturation math, sweep framework, identity bridge. Education-specific, excluded: eligibility/fit, course detectors, `re_classify_eligibility`, Zoho/Clay/Apify actions, theme names, point values, segment thresholds, Dubai tick schedule (ADX uses its own reporting day).

## Schema (`lib/voice/db.ts`)

`voice_calls` (id, lead_id, mode, direction, initiated_by, twilio_call_sid, agent_session_id, from/to_number, status CHECK of 8 values, duration_seconds, recording_sid/url, detected_language, summary JSON, outcome, outcome_confirmed_by, started_at, ended_at, review_status, twilio_region, transcription_failed_at/error, archive_key, archived_at, recording_deleted_at). `voice_transcript_segments` (call_id FK cascade, seq, speaker CHECK lead/rep/agent, text, start_ms, end_ms, is_final). `voice_numbers` caller-ID pool.

## Environment variable names (names only)

Twilio: `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_API_KEY`, `TWILIO_API_SECRET`, `TWILIO_TWIML_APP_SID`, `TWILIO_VOICE_API_KEY_SID`, `TWILIO_VOICE_API_KEY_SECRET`, `TWILIO_VOICE_NUMBERS`, `TWILIO_WEBHOOK_SECRET`. ElevenLabs: `ELEVENLABS_API_KEY`. Deepgram: `DEEPGRAM_API_KEY`, `DEEPGRAM_MODEL`, `DEEPGRAM_LANGUAGE`, `DEEPGRAM_KEYTERMS`. Voice: `VOICE_AGENT_PROVIDER`, `VOICE_TRANSCRIPTION_PROVIDER`, `VOICE_AGENT_VERTICAL`, `VOICE_AGENT_MAX_CONCURRENT_CALLS`, `VOICE_RECORDING_RETENTION_DAYS`. ADX uses its own `ADX_`-prefixed names and file-based secrets.

## Reuse verdict

Port the concepts: the provider seam, call state machine, occurrence-key outcome application, µ-law framing, pacing, clear-on-interrupt, mark-based hangup, timeouts, and the Twilio status mapping. Exclude every education rule. Add `X-Twilio-Signature` validation. ADX's telecom route for the UAE is not chosen; Twilio is a reference transport whose live use stays blocked until the owner picks and authorizes a route (B4.4).
