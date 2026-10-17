-- +goose Up
-- Server-wide model usage per UTC day and model, kept so the daily cap
-- survives a restart. No workspace: the cap is the server's bill.
CREATE TABLE model_usage_days (
    day           TEXT    NOT NULL,
    model         TEXT    NOT NULL,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    calls         INTEGER NOT NULL DEFAULT 0,
    refused       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, model)
);

-- A model release is a candidate model evaluated against the team's labelled
-- calls. It is approved only by a person, and never when it reads worse than
-- the approved one.
CREATE TABLE model_releases (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    model        TEXT     NOT NULL,
    state        TEXT     NOT NULL CHECK (state IN ('candidate', 'approved', 'rejected', 'superseded')),
    labels       INTEGER  NOT NULL DEFAULT 0,
    agreements   INTEGER  NOT NULL DEFAULT 0,
    agreement    REAL,
    note         TEXT     NOT NULL DEFAULT '',
    evaluated_by INTEGER  REFERENCES users (id),
    evaluated_at DATETIME NOT NULL,
    decided_by   INTEGER  REFERENCES users (id),
    decided_at   DATETIME,
    UNIQUE (workspace_id, model)
);

-- A load measurement records the conditions with the numbers, so no
-- throughput is ever quoted without them.
CREATE TABLE load_measurements (
    id          INTEGER  PRIMARY KEY,
    ran_at      DATETIME NOT NULL,
    host        TEXT     NOT NULL,
    cpus        INTEGER  NOT NULL,
    go_version  TEXT     NOT NULL,
    duration_ms INTEGER  NOT NULL,
    concurrency INTEGER  NOT NULL,
    operations  INTEGER  NOT NULL,
    errors      INTEGER  NOT NULL,
    p50_ms      REAL     NOT NULL,
    p95_ms      REAL     NOT NULL,
    max_ms      REAL     NOT NULL,
    mix         TEXT     NOT NULL CHECK (json_valid(mix)),
    notes       TEXT     NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE load_measurements;
DROP TABLE model_releases;
DROP TABLE model_usage_days;
