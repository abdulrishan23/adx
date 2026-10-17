-- +goose Up
-- A workspace is one client's ADX. Phase 0 has one, Reportage, with the fixed
-- ID 1 that core.DefaultWorkspace names. Multi-client provisioning comes in
-- phase 6; until then every workspace-owned row points here.
CREATE TABLE workspaces (
    id         INTEGER  PRIMARY KEY,
    slug       TEXT     NOT NULL UNIQUE,
    name       TEXT     NOT NULL,
    created_at DATETIME NOT NULL
);
INSERT INTO workspaces (id, slug, name, created_at) VALUES (1, 'reportage', 'Reportage', '2026-10-17 00:00:00');

-- Who may act in which workspace, and as what. While there is one workspace,
-- role mirrors users.role and core/users writes both in one transaction.
-- enabled follows users.status so a disabled person has no live membership.
CREATE TABLE memberships (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    user_id      INTEGER  NOT NULL REFERENCES users (id),
    role         TEXT     NOT NULL CHECK (role IN ('viewer', 'marketer', 'admin')),
    enabled      INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, user_id)
);
INSERT INTO memberships (workspace_id, user_id, role, enabled, created_at)
SELECT 1, id, role, CASE WHEN status = 'disabled' THEN 0 ELSE 1 END, created_at FROM users;

-- Web sessions. The cookie holds a random ID and this table only its hash,
-- so a copied database holds nothing anyone can sign in with. Every request
-- looks the session up, so disabling someone applies on their next click.
-- expires_at is the absolute end; idle expiry is last_seen_at plus the idle
-- limit, checked in core.
CREATE TABLE sessions (
    id_hash      TEXT     PRIMARY KEY,
    user_id      INTEGER  NOT NULL REFERENCES users (id),
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    via          TEXT     NOT NULL,
    created_at   DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL,
    expires_at   DATETIME NOT NULL,
    ended_at     DATETIME
);
CREATE INDEX sessions_user ON sessions (user_id);

-- One row per sign-in started and not yet finished. The state is hashed; the
-- PKCE verifier is kept so the callback can send it, and is useless without
-- the one-time code the provider issues.
CREATE TABLE sign_in_attempts (
    state_hash    TEXT     PRIMARY KEY,
    nonce_hash    TEXT     NOT NULL,
    code_verifier TEXT     NOT NULL,
    return_to     TEXT     NOT NULL,
    expires_at    DATETIME NOT NULL
);

-- +goose Down
DROP TABLE sign_in_attempts;
DROP TABLE sessions;
DROP TABLE memberships;
DROP TABLE workspaces;
