-- +goose Up
-- A research brief is one question about a brand or project: which pages to
-- read, which competitors to look at, which keyword seeds to size, whether
-- to audit our own account history. Running it fetches each item once and
-- records what came back as evidence and findings (core/research).
CREATE TABLE research_briefs (
    id                     INTEGER  PRIMARY KEY,
    workspace_id           INTEGER  NOT NULL REFERENCES workspaces (id),
    brand_id               INTEGER  NOT NULL REFERENCES brands (id),
    project_id             INTEGER  REFERENCES projects (id),
    title                  TEXT     NOT NULL,
    question               TEXT     NOT NULL DEFAULT '',
    geography              TEXT     NOT NULL,
    language               TEXT     NOT NULL,
    google_ads_account_id  INTEGER  REFERENCES accounts (id),
    created_by             INTEGER  REFERENCES users (id),
    created_at             DATETIME NOT NULL
);
CREATE INDEX research_briefs_brand ON research_briefs (workspace_id, brand_id);

-- One retrieval. A competitor item whose identity is not settled waits in
-- needs_identity and is never fetched. A failure keeps its safe message and
-- produces no finding: an unavailable source is an unavailable source.
CREATE TABLE research_items (
    id                   INTEGER  PRIMARY KEY,
    workspace_id         INTEGER  NOT NULL REFERENCES workspaces (id),
    brief_id             INTEGER  NOT NULL REFERENCES research_briefs (id),
    kind                 TEXT     NOT NULL CHECK (kind IN ('web_page', 'competitor_page', 'keywords', 'account_audit')),
    target               TEXT     NOT NULL,
    competitor_name      TEXT     NOT NULL DEFAULT '',
    competitor_brand_id  INTEGER  REFERENCES brands (id),
    identity_state       TEXT     NOT NULL CHECK (identity_state IN ('n/a', 'resolved', 'ambiguous', 'unknown')),
    candidates           TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(candidates)),
    state                TEXT     NOT NULL CHECK (state IN ('pending', 'needs_identity', 'fetched', 'failed', 'refused')),
    error_safe           TEXT     NOT NULL DEFAULT '',
    attempts             INTEGER  NOT NULL DEFAULT 0,
    fetched_at           DATETIME,
    run_id               INTEGER  REFERENCES runs (id),
    evidence_id          INTEGER  REFERENCES evidence (id),
    created_at           DATETIME NOT NULL
);
CREATE INDEX research_items_brief ON research_items (brief_id, state);

-- What an item yielded. Scope keeps our account's figures ('account') apart
-- from anything read off the public web ('public'); class keeps counts
-- apart from estimates. Nothing here is a model's output.
CREATE TABLE research_findings (
    id                  INTEGER  PRIMARY KEY,
    workspace_id        INTEGER  NOT NULL REFERENCES workspaces (id),
    brief_id            INTEGER  NOT NULL REFERENCES research_briefs (id),
    item_id             INTEGER  NOT NULL REFERENCES research_items (id),
    scope               TEXT     NOT NULL CHECK (scope IN ('account', 'public')),
    subject_brand_id    INTEGER  REFERENCES brands (id),
    kind                TEXT     NOT NULL CHECK (kind IN ('page_text', 'keyword', 'audit_metric', 'audit_note')),
    class               TEXT     NOT NULL CHECK (class IN ('observed', 'estimated', 'inferred')),
    label               TEXT     NOT NULL,
    payload             TEXT     NOT NULL CHECK (json_valid(payload)),
    source_url          TEXT     NOT NULL DEFAULT '',
    source_observed_at  DATETIME NOT NULL,
    geography           TEXT     NOT NULL,
    evidence_id         INTEGER  REFERENCES evidence (id),
    created_at          DATETIME NOT NULL
);
CREATE INDEX research_findings_brief ON research_findings (brief_id, scope, kind);

-- +goose Down
DROP TABLE research_findings;
DROP TABLE research_items;
DROP TABLE research_briefs;
