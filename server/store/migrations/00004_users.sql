-- +goose Up
-- People who may use ADX. Adding someone creates or finds their authentik
-- account, and ADX keeps that account's IDs. Sign-in matches on the UUID,
-- which authentik sends as the subject, so a changed email can't move an
-- account to someone else. Nobody is deleted, so the audit log always points
-- at a row.
CREATE TABLE users (
    id              INTEGER  PRIMARY KEY,
    email           TEXT     NOT NULL UNIQUE COLLATE NOCASE,
    name            TEXT,
    role            TEXT     NOT NULL CHECK (role IN ('viewer', 'marketer', 'admin')),
    status          TEXT     NOT NULL CHECK (status IN ('invited', 'active', 'disabled')),
    authentik_pk    INTEGER  NOT NULL UNIQUE,
    authentik_uuid  TEXT     NOT NULL UNIQUE,
    created_at      DATETIME NOT NULL,
    last_sign_in_at DATETIME
);

-- One row per sign-in, role change, MCP call and the like. A null actor means
-- ADX itself: the CLI, a job, or someone who isn't a user yet.
CREATE TABLE audit_log (
    id             INTEGER  PRIMARY KEY,
    at             DATETIME NOT NULL,
    actor_user_id  INTEGER  REFERENCES users (id),
    via            TEXT     NOT NULL,
    action         TEXT     NOT NULL,
    target_user_id INTEGER  REFERENCES users (id),
    detail         TEXT     NOT NULL DEFAULT '{}'
);

CREATE INDEX audit_log_at ON audit_log (at);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE users;
