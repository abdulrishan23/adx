-- +goose NO TRANSACTION
-- +goose Up
-- Accounts and projects become workspace-scoped. Both tables are rebuilt so
-- the uniqueness of a provider id or a project name holds within a
-- workspace, not across the whole database: two clients may both have a
-- Google Ads account "123" or a project called "The Wadi" and never see
-- each other's. Existing rows belong to workspace 1 (Reportage).
PRAGMA foreign_keys = OFF;

CREATE TABLE accounts_new (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL DEFAULT 1 REFERENCES workspaces (id),
    type          TEXT     NOT NULL,
    external_id   TEXT     NOT NULL,
    credential_id INTEGER  NOT NULL REFERENCES credentials (id),
    parent_id     TEXT,
    name          TEXT,
    currency      TEXT,
    time_zone     TEXT,
    details       TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(details)),
    connected_at  DATETIME NOT NULL,
    verified_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, type, external_id)
);
INSERT INTO accounts_new (id, workspace_id, type, external_id, credential_id, parent_id, name, currency, time_zone, details, connected_at, verified_at)
SELECT id, 1, type, external_id, credential_id, parent_id, name, currency, time_zone, details, connected_at, verified_at FROM accounts;
DROP TABLE accounts;
ALTER TABLE accounts_new RENAME TO accounts;
CREATE INDEX accounts_credential ON accounts (credential_id);
CREATE INDEX accounts_workspace ON accounts (workspace_id, type);

CREATE TABLE projects_new (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL DEFAULT 1 REFERENCES workspaces (id),
    name         TEXT     NOT NULL COLLATE NOCASE,
    created_at   DATETIME NOT NULL,
    brand_id     INTEGER  REFERENCES brands (id),
    UNIQUE (workspace_id, name)
);
INSERT INTO projects_new (id, workspace_id, name, created_at, brand_id)
SELECT id, 1, name, created_at, brand_id FROM projects;
DROP TABLE projects;
ALTER TABLE projects_new RENAME TO projects;

-- Industry configuration: versioned per workspace, never edited in place,
-- so one client's history keeps the definitions it was built with when
-- another client (or a later version) changes them.
CREATE TABLE industry_configs (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);
-- Reportage's configuration as built: off-plan residential real estate in
-- Dubai, the stages of docs/reference.md. Version 1 is what every assessment
-- so far was made under, and it never changes.
INSERT INTO industry_configs (workspace_id, version, document, created_by, created_at) VALUES (1, 1,
    '{"industry":"real_estate","vocabulary":{"enquiry":"a person who asked about a project","unit":"an apartment, townhouse or villa in a project","off-plan":"sold before completion"},"stages":["new","contacted","qualified","viewing","booked","lost"],"qualifiers":["budget stated","timeline within 6 months","residency or investor intent known"],"notes":""}',
    NULL, '2026-10-17 00:00:00');

PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;
DROP TABLE industry_configs;
CREATE TABLE projects_old (
    id         INTEGER PRIMARY KEY,
    name       TEXT     NOT NULL UNIQUE COLLATE NOCASE,
    created_at DATETIME NOT NULL,
    brand_id   INTEGER REFERENCES brands (id)
);
INSERT INTO projects_old (id, name, created_at, brand_id) SELECT id, name, created_at, brand_id FROM projects WHERE workspace_id = 1;
DROP TABLE projects;
ALTER TABLE projects_old RENAME TO projects;
CREATE TABLE accounts_old (
    id            INTEGER  PRIMARY KEY,
    type          TEXT     NOT NULL,
    external_id   TEXT     NOT NULL,
    credential_id INTEGER  NOT NULL REFERENCES credentials (id),
    parent_id     TEXT,
    name          TEXT,
    currency      TEXT,
    time_zone     TEXT,
    details       TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(details)),
    connected_at  DATETIME NOT NULL,
    verified_at   DATETIME NOT NULL,
    UNIQUE (type, external_id)
);
INSERT INTO accounts_old (id, type, external_id, credential_id, parent_id, name, currency, time_zone, details, connected_at, verified_at)
SELECT id, type, external_id, credential_id, parent_id, name, currency, time_zone, details, connected_at, verified_at FROM accounts WHERE workspace_id = 1;
DROP TABLE accounts;
ALTER TABLE accounts_old RENAME TO accounts;
CREATE INDEX accounts_credential ON accounts (credential_id);
PRAGMA foreign_keys = ON;
