-- +goose Up
-- The calling policy: which telecom route is chosen and who authorized it,
-- consent, hours, capacity, attempts and recording retention. No row means
-- no live call, ever; the fake route in tests is a route like any other.
CREATE TABLE call_policies (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

-- A call is one attempt to reach one person about one enquiry. The
-- occurrence key is unique, so two requests for the same attempt are one
-- call. State follows the reference source's machine plus `reserved` (row
-- written, provider not yet asked) and `unknown` (provider asked, answer
-- lost), which must be reconciled before another attempt is placed.
CREATE TABLE calls (
    id                    INTEGER  PRIMARY KEY,
    workspace_id          INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id             INTEGER  NOT NULL REFERENCES people (id),
    enquiry_id            INTEGER  NOT NULL REFERENCES enquiries (id),
    occurrence_key        TEXT     NOT NULL,
    mode                  TEXT     NOT NULL CHECK (mode IN ('qualification', 'callback')),
    direction             TEXT     NOT NULL DEFAULT 'outbound',
    to_number             TEXT     NOT NULL,
    from_number           TEXT     NOT NULL,
    state                 TEXT     NOT NULL CHECK (state IN ('reserved', 'queued', 'ringing', 'in_progress', 'completed', 'no_answer', 'busy', 'failed', 'canceled', 'unknown')),
    reason                TEXT     NOT NULL DEFAULT '',
    provider              TEXT     NOT NULL DEFAULT '',
    provider_call_id      TEXT     NOT NULL DEFAULT '',
    policy_version        INTEGER  NOT NULL,
    consent_basis         TEXT     NOT NULL,
    attempt               INTEGER  NOT NULL,
    record                INTEGER  NOT NULL DEFAULT 0,
    recording_ref         TEXT     NOT NULL DEFAULT '',
    recording_deleted_at  DATETIME,
    duration_seconds      INTEGER,
    requested_by          INTEGER  REFERENCES users (id),
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL,
    started_at            DATETIME,
    ended_at              DATETIME,
    unknown_since         DATETIME,
    UNIQUE (workspace_id, occurrence_key)
);
CREATE INDEX calls_active ON calls (workspace_id, state);
CREATE INDEX calls_enquiry ON calls (enquiry_id, created_at);
CREATE INDEX calls_provider ON calls (provider, provider_call_id);

-- Every provider event and every ADX transition, with the provider's own
-- id when it sends one so a redelivered callback is one event.
CREATE TABLE call_events (
    id           INTEGER  PRIMARY KEY,
    call_id      INTEGER  NOT NULL REFERENCES calls (id),
    kind         TEXT     NOT NULL,
    from_state   TEXT     NOT NULL,
    to_state     TEXT     NOT NULL,
    detail       TEXT     NOT NULL DEFAULT '',
    payload      TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
    event_key    TEXT     NOT NULL,
    received_at  DATETIME NOT NULL,
    UNIQUE (call_id, event_key)
);

-- +goose Down
DROP TABLE call_events;
DROP TABLE calls;
DROP TABLE call_policies;
