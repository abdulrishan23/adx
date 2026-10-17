-- +goose Up
-- What ADX has proven it can do through an account, and when. A credential
-- existing says nothing about permission: a read is verified by a successful
-- read, a write by a configured policy plus a verified write probe, and both
-- go stale as verified_at ages. last_error is a safe message, never a body.
CREATE TABLE account_capabilities (
    id          INTEGER  PRIMARY KEY,
    account_id  INTEGER  NOT NULL REFERENCES accounts (id),
    capability  TEXT     NOT NULL,
    state       TEXT     NOT NULL CHECK (state IN ('unknown', 'verified', 'failed', 'needs_permission')),
    verified_at DATETIME,
    checked_at  DATETIME NOT NULL,
    last_error  TEXT,
    UNIQUE (account_id, capability)
);

-- The permission checklist, caps and per-account write enablement, as one
-- versioned JSON document per workspace (D-13). No row means unconfigured,
-- and unconfigured denies every external write. Documents are never edited:
-- a change is a new version, so a past decision can name the version it was
-- made under.
CREATE TABLE policies (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

-- Kill switches stop every external write for an account, or for the whole
-- workspace when account_id is 0. They block undispatched actions; they can't
-- undo a call already made.
CREATE TABLE kill_switches (
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    account_id   INTEGER  NOT NULL DEFAULT 0,
    engaged      INTEGER  NOT NULL CHECK (engaged IN (0, 1)),
    reason       TEXT     NOT NULL DEFAULT '',
    set_by       INTEGER  REFERENCES users (id),
    set_at       DATETIME NOT NULL,
    PRIMARY KEY (workspace_id, account_id)
);

-- +goose Down
DROP TABLE kill_switches;
DROP TABLE policies;
DROP TABLE account_capabilities;
