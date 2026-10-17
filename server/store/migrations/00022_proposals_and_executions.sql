-- +goose Up
-- Spend authority. An envelope is the money an accepted plan version allows
-- for one allocation; a reservation takes part of it for one proposal and
-- is checked against the envelope inside a single write transaction, so
-- two reservations can never both take the same remaining amount.
CREATE TABLE budget_envelopes (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    plan_id       INTEGER  NOT NULL REFERENCES plans (id),
    plan_version  INTEGER  NOT NULL,
    channel       TEXT     NOT NULL,
    account_id    INTEGER  REFERENCES accounts (id),
    currency      TEXT     NOT NULL,
    amount_minor  INTEGER  NOT NULL,
    opened_by     INTEGER  REFERENCES users (id),
    opened_at     DATETIME NOT NULL,
    UNIQUE (plan_id, plan_version, channel)
);

CREATE TABLE budget_reservations (
    id            INTEGER  PRIMARY KEY,
    envelope_id   INTEGER  NOT NULL REFERENCES budget_envelopes (id),
    proposal_id   INTEGER  NOT NULL,
    amount_minor  INTEGER  NOT NULL,
    state         TEXT     NOT NULL CHECK (state IN ('reserved', 'consumed', 'released')),
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL
);
CREATE INDEX budget_reservations_envelope ON budget_reservations (envelope_id, state);

-- A proposal is one typed change to an outside account: what it is now,
-- what it should become, why, under which policy version. Its target
-- carries a version; an approval binds to that version, and a changed
-- target makes the proposal stale rather than letting an old approval
-- carry a new change.
CREATE TABLE proposals (
    id               INTEGER  PRIMARY KEY,
    workspace_id     INTEGER  NOT NULL REFERENCES workspaces (id),
    account_id       INTEGER  NOT NULL REFERENCES accounts (id),
    action           TEXT     NOT NULL,
    target_kind      TEXT     NOT NULL,
    target_ref       TEXT     NOT NULL,
    target_version   TEXT     NOT NULL,
    expected_current TEXT     NOT NULL CHECK (json_valid(expected_current)),
    proposed         TEXT     NOT NULL CHECK (json_valid(proposed)),
    reason           TEXT     NOT NULL,
    evidence_refs    TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(evidence_refs)),
    amount_minor     INTEGER,
    currency         TEXT,
    envelope_id      INTEGER  REFERENCES budget_envelopes (id),
    policy_version   INTEGER,
    policy_outcome   TEXT     NOT NULL,
    policy_reason    TEXT     NOT NULL,
    state            TEXT     NOT NULL CHECK (state IN ('denied', 'proposed', 'approved', 'rejected', 'stale', 'executing', 'done', 'failed', 'cancelled')),
    idempotency_key  TEXT     NOT NULL,
    proposed_by      INTEGER  REFERENCES users (id),
    proposed_via     TEXT     NOT NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX proposals_target ON proposals (workspace_id, target_kind, target_ref, state);
CREATE INDEX proposals_state ON proposals (workspace_id, state, created_at);

CREATE TABLE approvals (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    proposal_id     INTEGER  NOT NULL REFERENCES proposals (id),
    target_version  TEXT     NOT NULL,
    decision        TEXT     NOT NULL CHECK (decision IN ('approve', 'reject')),
    policy_version  INTEGER,
    note            TEXT     NOT NULL DEFAULT '',
    decided_by      INTEGER  NOT NULL REFERENCES users (id),
    decided_at      DATETIME NOT NULL
);
CREATE INDEX approvals_proposal ON approvals (proposal_id);

-- The outbox. An execution intent is written in the same transaction as
-- its reservation and nothing talks to a provider inside that transaction.
-- A worker (B3.5) picks intents up, re-checks authority and the kill
-- switch, dispatches, and records what the provider said.
CREATE TABLE execution_intents (
    id               INTEGER  PRIMARY KEY,
    workspace_id     INTEGER  NOT NULL REFERENCES workspaces (id),
    proposal_id      INTEGER  NOT NULL REFERENCES proposals (id),
    account_id       INTEGER  NOT NULL REFERENCES accounts (id),
    idempotency_key  TEXT     NOT NULL,
    reservation_id   INTEGER  REFERENCES budget_reservations (id),
    state            TEXT     NOT NULL CHECK (state IN ('requested', 'authorized', 'reserved', 'dispatching', 'submitted', 'externally_confirmed', 'uncertain', 'reconciling', 'confirmed', 'failed', 'cancelled', 'compensated')),
    attempts         INTEGER  NOT NULL DEFAULT 0,
    provider_refs    TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(provider_refs)),
    last_error_safe  TEXT     NOT NULL DEFAULT '',
    reconcile_after  DATETIME,
    authorized_by    INTEGER  REFERENCES users (id),
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    dispatched_at    DATETIME,
    confirmed_at     DATETIME,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX execution_intents_state ON execution_intents (workspace_id, state, updated_at);

-- +goose Down
DROP TABLE execution_intents;
DROP TABLE approvals;
DROP TABLE proposals;
DROP TABLE budget_reservations;
DROP TABLE budget_envelopes;
