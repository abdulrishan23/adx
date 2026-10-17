-- +goose Up
-- Learning records tie a recommendation to what a person decided (adopted,
-- modified, ignored), the action that followed, and the outcomes observed
-- later. Outcomes attach to the record whose action window covers them, so
-- an improvement lands on the intervention that preceded it, never on advice
-- nobody took. Insights are statements with a class: hypothesis until
-- enough observed outcomes support them, validated only with a controlled
-- comparison; a model agreeing is not evidence.
CREATE TABLE learning_records (
    id                 INTEGER  PRIMARY KEY,
    workspace_id       INTEGER  NOT NULL REFERENCES workspaces (id),
    recommendation_ref TEXT     NOT NULL,
    source             TEXT     NOT NULL,
    subject_kind       TEXT     NOT NULL,
    subject_ref        TEXT     NOT NULL,
    recommended        TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(recommended)),
    decision           TEXT     NOT NULL CHECK (decision IN ('pending', 'adopted', 'modified', 'ignored')),
    decision_note      TEXT     NOT NULL DEFAULT '',
    decided_by         INTEGER  REFERENCES users (id),
    decided_at         DATETIME,
    action_ref         TEXT     NOT NULL DEFAULT '',
    action_at          DATETIME,
    window_days        INTEGER  NOT NULL DEFAULT 14,
    created_at         DATETIME NOT NULL,
    UNIQUE (workspace_id, recommendation_ref)
);
CREATE INDEX learning_records_subject ON learning_records (workspace_id, subject_kind, subject_ref, action_at);

CREATE TABLE learning_outcomes (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    record_id    INTEGER  REFERENCES learning_records (id),
    subject_kind TEXT     NOT NULL,
    subject_ref  TEXT     NOT NULL,
    metric       TEXT     NOT NULL,
    value        REAL     NOT NULL,
    unit         TEXT     NOT NULL DEFAULT '',
    class        TEXT     NOT NULL CHECK (class IN ('observed', 'attributed', 'control')),
    observed_at  DATETIME NOT NULL,
    evidence_id  INTEGER  REFERENCES evidence (id),
    note         TEXT     NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL
);
CREATE INDEX learning_outcomes_record ON learning_outcomes (record_id);
CREATE INDEX learning_outcomes_subject ON learning_outcomes (workspace_id, subject_kind, subject_ref, observed_at);

CREATE TABLE insights (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    lineage        INTEGER  NOT NULL,
    version        INTEGER  NOT NULL DEFAULT 1,
    statement      TEXT     NOT NULL,
    scope          TEXT     NOT NULL DEFAULT '',
    class          TEXT     NOT NULL CHECK (class IN ('hypothesis', 'supported', 'validated', 'retired')),
    causal         INTEGER  NOT NULL DEFAULT 0,
    support        TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(support)),
    source         TEXT     NOT NULL CHECK (source IN ('human', 'model')),
    control_evidence_id INTEGER REFERENCES evidence (id),
    created_by     INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    promoted_by    INTEGER  REFERENCES users (id),
    promoted_at    DATETIME,
    retired_at     DATETIME,
    UNIQUE (workspace_id, lineage, version)
);
CREATE INDEX insights_class ON insights (workspace_id, class);

-- +goose Down
DROP TABLE insights;
DROP TABLE learning_outcomes;
DROP TABLE learning_records;
