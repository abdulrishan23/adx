-- +goose Up
-- The native pipeline: one opportunity per enquiry (a person with two
-- enquiries has two), owned stages, assignments, interactions and follow-up
-- tasks. Configuration is versioned; opportunities carry a version for
-- optimistic concurrency so two people editing one record collide loudly.
CREATE TABLE pipeline_configs (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

CREATE TABLE opportunities (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    enquiry_id     INTEGER  NOT NULL UNIQUE REFERENCES enquiries (id),
    person_id      INTEGER  NOT NULL REFERENCES people (id),
    project_id     INTEGER  REFERENCES projects (id),
    stage          TEXT     NOT NULL,
    stage_set_by   TEXT     NOT NULL CHECK (stage_set_by IN ('user', 'import', 'system')),
    state          TEXT     NOT NULL CHECK (state IN ('open', 'won', 'lost')),
    value_minor    INTEGER,
    currency       TEXT,
    owner_user_id  INTEGER  REFERENCES users (id),
    version        INTEGER  NOT NULL DEFAULT 1,
    created_by     INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    closed_at      DATETIME
);
CREATE INDEX opportunities_stage ON opportunities (workspace_id, state, stage);
CREATE INDEX opportunities_person ON opportunities (person_id);

CREATE TABLE opportunity_assignments (
    id             INTEGER  PRIMARY KEY,
    opportunity_id INTEGER  NOT NULL REFERENCES opportunities (id),
    user_id        INTEGER  NOT NULL REFERENCES users (id),
    assigned_by    INTEGER  REFERENCES users (id),
    reason         TEXT     NOT NULL DEFAULT '',
    assigned_at    DATETIME NOT NULL,
    ended_at       DATETIME
);
CREATE INDEX opportunity_assignments_active ON opportunity_assignments (opportunity_id, ended_at);

CREATE TABLE interactions (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id      INTEGER  NOT NULL REFERENCES people (id),
    opportunity_id INTEGER  REFERENCES opportunities (id),
    kind           TEXT     NOT NULL CHECK (kind IN ('call', 'meeting', 'message', 'note', 'visit', 'email')),
    summary        TEXT     NOT NULL,
    occurred_at    DATETIME NOT NULL,
    recorded_by    INTEGER  REFERENCES users (id),
    evidence_id    INTEGER  REFERENCES evidence (id),
    created_at     DATETIME NOT NULL
);
CREATE INDEX interactions_person ON interactions (person_id, occurred_at);

CREATE TABLE follow_up_tasks (
    id               INTEGER  PRIMARY KEY,
    workspace_id     INTEGER  NOT NULL REFERENCES workspaces (id),
    opportunity_id   INTEGER  REFERENCES opportunities (id),
    person_id        INTEGER  NOT NULL REFERENCES people (id),
    title            TEXT     NOT NULL,
    due_at           DATETIME,
    state            TEXT     NOT NULL CHECK (state IN ('open', 'done', 'canceled')),
    assignee_user_id INTEGER  REFERENCES users (id),
    occurrence_key   TEXT     NOT NULL,
    created_by       INTEGER  REFERENCES users (id),
    created_at       DATETIME NOT NULL,
    done_at          DATETIME,
    done_by          INTEGER  REFERENCES users (id),
    UNIQUE (workspace_id, occurrence_key)
);
CREATE INDEX follow_up_tasks_open ON follow_up_tasks (workspace_id, state, due_at);

-- Import batches: a preview computes rows and writes nothing else; apply
-- walks the rows from the cursor, one transaction each, so a crash resumes
-- where it stopped and a repeated apply skips applied rows.
CREATE TABLE crm_import_batches (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    source         TEXT     NOT NULL,
    state          TEXT     NOT NULL CHECK (state IN ('previewed', 'applying', 'applied', 'failed')),
    config_version INTEGER  NOT NULL,
    cursor         INTEGER  NOT NULL DEFAULT 0,
    total          INTEGER  NOT NULL DEFAULT 0,
    created_n      INTEGER  NOT NULL DEFAULT 0,
    updated_n      INTEGER  NOT NULL DEFAULT 0,
    skipped_n      INTEGER  NOT NULL DEFAULT 0,
    conflict_n     INTEGER  NOT NULL DEFAULT 0,
    error          TEXT     NOT NULL DEFAULT '',
    created_by     INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    applied_at     DATETIME
);

CREATE TABLE crm_import_rows (
    id         INTEGER  PRIMARY KEY,
    batch_id   INTEGER  NOT NULL REFERENCES crm_import_batches (id),
    position   INTEGER  NOT NULL,
    enquiry_id INTEGER  NOT NULL REFERENCES enquiries (id),
    lead_id    INTEGER  REFERENCES leads (id),
    action     TEXT     NOT NULL CHECK (action IN ('create', 'update_stage', 'skip', 'conflict')),
    detail     TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(detail)),
    applied_at DATETIME,
    UNIQUE (batch_id, position)
);

-- Stable mappings between local records and a provider's ids, both ways
-- unique, so a re-import or an export lands on the same record.
CREATE TABLE provider_mappings (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    provider     TEXT     NOT NULL,
    kind         TEXT     NOT NULL,
    local_id     INTEGER  NOT NULL,
    external_id  TEXT     NOT NULL,
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, provider, kind, external_id),
    UNIQUE (workspace_id, provider, kind, local_id)
);

-- +goose Down
DROP TABLE provider_mappings;
DROP TABLE crm_import_rows;
DROP TABLE crm_import_batches;
DROP TABLE follow_up_tasks;
DROP TABLE interactions;
DROP TABLE opportunity_assignments;
DROP TABLE opportunities;
DROP TABLE pipeline_configs;
