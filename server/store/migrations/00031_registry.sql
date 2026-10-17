-- +goose Up
-- The registry of what agents may do: skills (instructions with typed
-- input and output, an evidence scope and permitted tools), agents (skills
-- plus an account scope, never above marketer) and workflow definitions
-- (a bounded graph of steps). Every version is immutable; a new version is
-- a draft until it is activated, and a version that widens access or came
-- from a model needs an admin's acceptance.
CREATE TABLE registry_entries (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    kind         TEXT     NOT NULL CHECK (kind IN ('skill', 'agent', 'workflow')),
    key          TEXT     NOT NULL,
    version      INTEGER  NOT NULL,
    state        TEXT     NOT NULL CHECK (state IN ('draft', 'active', 'retired')),
    source       TEXT     NOT NULL CHECK (source IN ('user', 'model')),
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    validation   TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(validation)),
    needs_admin  INTEGER  NOT NULL DEFAULT 0,
    reason       TEXT     NOT NULL DEFAULT '',
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    activated_by INTEGER  REFERENCES users (id),
    activated_at DATETIME,
    retired_at   DATETIME,
    UNIQUE (workspace_id, kind, key, version)
);
CREATE INDEX registry_entries_active ON registry_entries (workspace_id, kind, key, state);

-- +goose Down
DROP TABLE registry_entries;
