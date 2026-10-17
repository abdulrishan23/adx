-- +goose Up
-- Landing pages are structured documents (core/pages.PageDocument), never
-- raw HTML: the server renders them with escaping and a strict content
-- security policy, so nothing an editor types can run as script. Versions
-- are immutable; a deployment points at exactly one version, and a rollback
-- is a new deployment pointing at the old one.
CREATE TABLE pages (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    brand_id      INTEGER  NOT NULL REFERENCES brands (id),
    project_id    INTEGER  REFERENCES projects (id),
    plan_id       INTEGER  REFERENCES plans (id),
    plan_version  INTEGER,
    slug          TEXT     NOT NULL,
    name          TEXT     NOT NULL,
    language      TEXT     NOT NULL CHECK (language IN ('en', 'ar')),
    test_token    TEXT     NOT NULL,
    created_by    INTEGER  REFERENCES users (id),
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    UNIQUE (workspace_id, slug)
);

CREATE TABLE page_versions (
    id          INTEGER  PRIMARY KEY,
    page_id     INTEGER  NOT NULL REFERENCES pages (id),
    version     INTEGER  NOT NULL,
    document    TEXT     NOT NULL CHECK (json_valid(document)),
    note        TEXT     NOT NULL DEFAULT '',
    created_by  INTEGER  REFERENCES users (id),
    created_at  DATETIME NOT NULL,
    UNIQUE (page_id, version)
);

CREATE TABLE page_deployments (
    id                  INTEGER  PRIMARY KEY,
    page_id             INTEGER  NOT NULL REFERENCES pages (id),
    version_id          INTEGER  NOT NULL REFERENCES page_versions (id),
    adapter             TEXT     NOT NULL,
    state               TEXT     NOT NULL CHECK (state IN ('published', 'failed', 'superseded')),
    url                 TEXT     NOT NULL DEFAULT '',
    output_sha256       TEXT     NOT NULL DEFAULT '',
    rolled_back_from_id INTEGER  REFERENCES page_deployments (id),
    error_safe          TEXT     NOT NULL DEFAULT '',
    deployed_by         INTEGER  REFERENCES users (id),
    deployed_at         DATETIME NOT NULL
);
CREATE INDEX page_deployments_page ON page_deployments (page_id, deployed_at);

-- A form submission is scoped to the page version that received it and to
-- the page's project. test marks the team's own test sends; they never
-- become customer events. ip_hash is a salted hash for rate handling only.
CREATE TABLE form_submissions (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    page_id       INTEGER  NOT NULL REFERENCES pages (id),
    version_id    INTEGER  NOT NULL REFERENCES page_versions (id),
    project_id    INTEGER  REFERENCES projects (id),
    state         TEXT     NOT NULL CHECK (state IN ('received', 'rejected')),
    reason        TEXT     NOT NULL DEFAULT '',
    test          INTEGER  NOT NULL DEFAULT 0 CHECK (test IN (0, 1)),
    fields        TEXT     NOT NULL CHECK (json_valid(fields)),
    attribution   TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(attribution)),
    ip_hash       TEXT     NOT NULL DEFAULT '',
    user_agent    TEXT     NOT NULL DEFAULT '',
    evidence_id   INTEGER  REFERENCES evidence (id),
    received_at   DATETIME NOT NULL
);
CREATE INDEX form_submissions_page ON form_submissions (page_id, received_at);

-- +goose Down
DROP TABLE form_submissions;
DROP TABLE page_deployments;
DROP TABLE page_versions;
DROP TABLE pages;
