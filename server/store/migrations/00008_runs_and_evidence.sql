-- +goose Up
-- A run is one durable piece of work: an import, a refresh, a report build.
-- occurrence_key names the work, not the attempt: "ghl.import" for location
-- X covering day D is one occurrence however many times it is retried, so a
-- repeat is reused instead of done twice. public_id is the opaque reference
-- the API hands out. completeness is what the run proved: partial reads can
-- never be marked complete.
CREATE TABLE runs (
    id               INTEGER  PRIMARY KEY,
    public_id        TEXT     NOT NULL UNIQUE,
    workspace_id     INTEGER  NOT NULL REFERENCES workspaces (id),
    kind             TEXT     NOT NULL,
    occurrence_key   TEXT     NOT NULL,
    account_id       INTEGER  REFERENCES accounts (id),
    state            TEXT     NOT NULL CHECK (state IN ('queued', 'running', 'succeeded', 'partial', 'failed', 'interrupted')),
    completeness     TEXT     NOT NULL DEFAULT 'unknown' CHECK (completeness IN ('unknown', 'complete', 'partial')),
    params           TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    progress         TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(progress)),
    error_safe       TEXT,
    attempt          INTEGER  NOT NULL DEFAULT 0,
    lease_owner      TEXT,
    lease_until      DATETIME,
    requested_by     INTEGER  REFERENCES users (id),
    created_at       DATETIME NOT NULL,
    started_at       DATETIME,
    finished_at      DATETIME,
    observed_through DATETIME,
    UNIQUE (workspace_id, kind, occurrence_key)
);
CREATE INDEX runs_state ON runs (state, lease_until);
CREATE INDEX runs_created ON runs (workspace_id, created_at);

-- One account is read or written by one run at a time, so two refreshes
-- can't race each other against a provider's rate limit or each other's
-- cursor. The lease expires, so a crashed worker can't hold it forever.
CREATE TABLE account_leases (
    account_id INTEGER  PRIMARY KEY REFERENCES accounts (id),
    run_id     INTEGER  NOT NULL REFERENCES runs (id),
    owner      TEXT     NOT NULL,
    until      DATETIME NOT NULL
);

-- Evidence is an observation ADX made: a contact as GoHighLevel returned it,
-- a day of campaign metrics, a figure it computed. The original is kept.
-- A later observation of the same thing with a different payload is a new
-- version, and the old row points at it. class says how far to trust it.
-- retain_until is null until a retention period is configured (nothing is
-- deleted automatically).
CREATE TABLE evidence (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    run_id          INTEGER  REFERENCES runs (id),
    account_id      INTEGER  REFERENCES accounts (id),
    source          TEXT     NOT NULL,
    kind            TEXT     NOT NULL,
    source_ref      TEXT     NOT NULL,
    scope           TEXT     NOT NULL DEFAULT '',
    class           TEXT     NOT NULL CHECK (class IN ('observed', 'estimated', 'inferred')),
    observed_at     DATETIME NOT NULL,
    event_at        DATETIME,
    payload_version INTEGER  NOT NULL,
    payload         TEXT     NOT NULL CHECK (json_valid(payload)),
    payload_hash    TEXT     NOT NULL,
    completeness    TEXT     NOT NULL CHECK (completeness IN ('complete', 'partial')),
    retain_until    DATETIME,
    superseded_by   INTEGER  REFERENCES evidence (id),
    UNIQUE (workspace_id, source, kind, source_ref, payload_version)
);
CREATE INDEX evidence_lookup ON evidence (workspace_id, source, kind, observed_at);
CREATE INDEX evidence_account ON evidence (account_id, kind);

-- +goose Down
DROP TABLE evidence;
DROP TABLE account_leases;
DROP TABLE runs;
