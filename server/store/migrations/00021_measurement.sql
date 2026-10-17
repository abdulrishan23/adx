-- +goose Up
-- A measurement event is one thing that happened that marketing counts: for
-- now, a form submission on an ADX page. source_ref makes each source row
-- one event, never two. test marks the team's own; counts is false for a
-- duplicate of an earlier event (same person, same page, same day) and for
-- tests, so nothing the team did to verify tracking is ever an enquiry.
CREATE TABLE measurement_events (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    kind            TEXT     NOT NULL,
    source          TEXT     NOT NULL,
    source_ref      TEXT     NOT NULL UNIQUE,
    page_id         INTEGER  REFERENCES pages (id),
    project_id      INTEGER  REFERENCES projects (id),
    plan_id         INTEGER  REFERENCES plans (id),
    test            INTEGER  NOT NULL DEFAULT 0 CHECK (test IN (0, 1)),
    dedupe_key      TEXT     NOT NULL,
    duplicate_of_id INTEGER  REFERENCES measurement_events (id),
    counts          INTEGER  NOT NULL CHECK (counts IN (0, 1)),
    attribution     TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(attribution)),
    occurred_at     DATETIME NOT NULL,
    recorded_at     DATETIME NOT NULL
);
CREATE INDEX measurement_events_dedupe ON measurement_events (workspace_id, dedupe_key, occurred_at);
CREATE INDEX measurement_events_page ON measurement_events (page_id, occurred_at);

-- A check is one verification the team ran against a plan version: a
-- controlled test submission, a conversion action that must exist, an
-- account's permission. Its state is passed, failed or unsupported (ADX
-- cannot verify this kind and says so). Evidence holds the detail.
CREATE TABLE measurement_checks (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    plan_id        INTEGER  NOT NULL REFERENCES plans (id),
    plan_version   INTEGER  NOT NULL,
    kind           TEXT     NOT NULL,
    page_id        INTEGER  REFERENCES pages (id),
    account_id     INTEGER  REFERENCES accounts (id),
    state          TEXT     NOT NULL CHECK (state IN ('passed', 'failed', 'unsupported')),
    detail         TEXT     NOT NULL DEFAULT '',
    submission_id  INTEGER  REFERENCES form_submissions (id),
    event_id       INTEGER  REFERENCES measurement_events (id),
    evidence_id    INTEGER  REFERENCES evidence (id),
    checked_by     INTEGER  REFERENCES users (id),
    checked_at     DATETIME NOT NULL
);
CREATE INDEX measurement_checks_plan ON measurement_checks (plan_id, plan_version, checked_at);

-- +goose Down
DROP TABLE measurement_checks;
DROP TABLE measurement_events;
