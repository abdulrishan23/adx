-- +goose Up
-- A thread (a conversation in core) is one person on one channel: what they wrote to ADX and
-- what ADX, the agent or a person, wrote back. The agent runs it while
-- state is 'agent'; 'waiting' is a handoff nobody has taken yet; 'person'
-- means someone took it; 'closed' ends it. The reading columns follow
-- call_sessions: the model's structured reading, how far it trusted it,
-- and the occurrence key it was applied under, so a re-read applies nothing
-- twice. A conversation is unique per workspace, channel and address, so a
-- second message from the same number lands on the same thread.
CREATE TABLE threads (
    id               INTEGER  PRIMARY KEY,
    workspace_id     INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id        INTEGER  NOT NULL REFERENCES people (id),
    enquiry_id       INTEGER  REFERENCES enquiries (id),
    channel          TEXT     NOT NULL CHECK (channel IN ('whatsapp')),
    address          TEXT     NOT NULL,
    state            TEXT     NOT NULL CHECK (state IN ('agent', 'waiting', 'person', 'closed')),
    state_reason     TEXT     NOT NULL DEFAULT '',
    handoff_kind     TEXT     NOT NULL DEFAULT '' CHECK (handoff_kind IN ('', 'requested_human', 'objection', 'outside_facts', 'stop', 'no_model', 'question')),
    handoff_at       DATETIME,
    language         TEXT     NOT NULL DEFAULT 'en',
    last_inbound_at  DATETIME,
    last_outbound_at DATETIME,
    summary          TEXT     NOT NULL DEFAULT '',
    answers          TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(answers)),
    outcome          TEXT     NOT NULL DEFAULT '',
    confidence       TEXT     NOT NULL DEFAULT '' CHECK (confidence IN ('', 'high', 'low')),
    requested_human  INTEGER  NOT NULL DEFAULT 0,
    review_status    TEXT     NOT NULL DEFAULT '' CHECK (review_status IN ('', 'auto', 'pending', 'confirmed', 'corrected')),
    review_reason    TEXT     NOT NULL DEFAULT '',
    read_at          DATETIME,
    read_turns       INTEGER  NOT NULL DEFAULT 0,
    applied_key      TEXT     NOT NULL DEFAULT '',
    applied_version  INTEGER  NOT NULL DEFAULT 0,
    applied_at       DATETIME,
    applied_detail   TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(applied_detail)),
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE (workspace_id, channel, address)
);
CREATE INDEX threads_state ON threads (workspace_id, state, updated_at);
CREATE INDEX threads_person ON threads (person_id);

-- A turn is one message either way. An inbound turn is 'received'. An
-- outbound turn starts as 'draft' (the agent's or a person's words, not yet
-- sent), moves to 'approved' when a person accepts it, 'sent' when the
-- adapter took it, then 'delivered' or 'read' from the provider's statuses;
-- 'held' names why it could not go (the channel is off, the window closed,
-- it quotes something not approved) and 'failed' is the provider's refusal.
-- idempotency_key is the provider's message id for inbound turns and the
-- draft's key for outbound ones, so a repeated delivery is the same turn.
CREATE TABLE thread_turns (
    id              INTEGER  PRIMARY KEY,
    thread_id       INTEGER  NOT NULL REFERENCES threads (id),
    direction       TEXT     NOT NULL CHECK (direction IN ('in', 'out')),
    author          TEXT     NOT NULL,
    kind            TEXT     NOT NULL CHECK (kind IN ('text', 'template', 'unsupported')),
    body            TEXT     NOT NULL,
    state           TEXT     NOT NULL CHECK (state IN ('received', 'draft', 'approved', 'held', 'sent', 'delivered', 'read', 'failed', 'discarded')),
    reason          TEXT     NOT NULL DEFAULT '',
    handoff         TEXT     NOT NULL DEFAULT '',
    provider_ref    TEXT     NOT NULL DEFAULT '',
    idempotency_key TEXT     NOT NULL,
    claims          TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(claims)),
    evidence_id     INTEGER  REFERENCES evidence (id),
    created_by      INTEGER  REFERENCES users (id),
    approved_by     INTEGER  REFERENCES users (id),
    occurred_at     DATETIME NOT NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    sent_at         DATETIME,
    UNIQUE (thread_id, idempotency_key)
);
CREATE INDEX thread_turns_conversation ON thread_turns (thread_id, id);
CREATE INDEX thread_turns_provider_ref ON thread_turns (provider_ref);

CREATE TABLE thread_corrections (
    id              INTEGER  PRIMARY KEY,
    thread_id       INTEGER  NOT NULL REFERENCES threads (id),
    outcome         TEXT     NOT NULL,
    answers         TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(answers)),
    note            TEXT     NOT NULL,
    created_by      INTEGER  REFERENCES users (id),
    created_at      DATETIME NOT NULL
);
CREATE INDEX thread_corrections_conversation ON thread_corrections (thread_id);

-- +goose Down
DROP TABLE thread_corrections;
DROP TABLE thread_turns;
DROP TABLE threads;
