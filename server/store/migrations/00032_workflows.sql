-- +goose Up
-- Workflow runs and their task graph. A task is one execution of a step
-- (iteration counts loop passes). Leases make a crashed worker's task
-- resumable; the operation key makes an external step's action happen once
-- however many times the task runs; input_hash ties a result to the
-- dependency outputs it was computed from, so a changed upstream result
-- invalidates what depended on it.
CREATE TABLE workflow_runs (
    id                 INTEGER  PRIMARY KEY,
    workspace_id       INTEGER  NOT NULL REFERENCES workspaces (id),
    definition_key     TEXT     NOT NULL,
    definition_version INTEGER  NOT NULL,
    state              TEXT     NOT NULL CHECK (state IN ('running', 'paused', 'blocked', 'done', 'failed', 'canceled')),
    params             TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    budget_tokens      INTEGER  NOT NULL DEFAULT 0,
    spent_tokens       INTEGER  NOT NULL DEFAULT 0,
    reason             TEXT     NOT NULL DEFAULT '',
    created_by         INTEGER  REFERENCES users (id),
    created_at         DATETIME NOT NULL,
    updated_at         DATETIME NOT NULL,
    finished_at        DATETIME
);
CREATE INDEX workflow_runs_state ON workflow_runs (workspace_id, state);

CREATE TABLE workflow_tasks (
    id             INTEGER  PRIMARY KEY,
    run_id         INTEGER  NOT NULL REFERENCES workflow_runs (id),
    key            TEXT     NOT NULL,
    iteration      INTEGER  NOT NULL DEFAULT 0,
    skill          TEXT     NOT NULL DEFAULT '',
    agent          TEXT     NOT NULL DEFAULT '',
    depends_on     TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(depends_on)),
    state          TEXT     NOT NULL CHECK (state IN ('pending', 'ready', 'leased', 'done', 'failed', 'blocked', 'invalidated', 'skipped')),
    attempt        INTEGER  NOT NULL DEFAULT 0,
    max_attempts   INTEGER  NOT NULL DEFAULT 3,
    lease_owner    TEXT,
    lease_until    DATETIME,
    input          TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(input)),
    resolved_input TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(resolved_input)),
    input_hash     TEXT     NOT NULL DEFAULT '',
    output         TEXT     NOT NULL DEFAULT 'null' CHECK (json_valid(output)),
    output_version INTEGER  NOT NULL DEFAULT 0,
    external       INTEGER  NOT NULL DEFAULT 0,
    operation_key  TEXT,
    external_ref   TEXT     NOT NULL DEFAULT '',
    loop_max       INTEGER  NOT NULL DEFAULT 0,
    budget_tokens  INTEGER  NOT NULL DEFAULT 0,
    spent_tokens   INTEGER  NOT NULL DEFAULT 0,
    reason         TEXT     NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    started_at     DATETIME,
    finished_at    DATETIME,
    UNIQUE (run_id, key, iteration)
);
CREATE UNIQUE INDEX workflow_tasks_operation ON workflow_tasks (operation_key) WHERE operation_key IS NOT NULL;
CREATE INDEX workflow_tasks_run_state ON workflow_tasks (run_id, state);

CREATE TABLE workflow_events (
    id      INTEGER  PRIMARY KEY,
    run_id  INTEGER  NOT NULL REFERENCES workflow_runs (id),
    task_id INTEGER  REFERENCES workflow_tasks (id),
    kind    TEXT     NOT NULL,
    detail  TEXT     NOT NULL DEFAULT '',
    at      DATETIME NOT NULL
);
CREATE INDEX workflow_events_run ON workflow_events (run_id, id);

-- +goose Down
DROP TABLE workflow_events;
DROP TABLE workflow_tasks;
DROP TABLE workflow_runs;
