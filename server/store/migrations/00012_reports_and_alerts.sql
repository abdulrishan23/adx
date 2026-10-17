-- +goose Up
-- A schedule says which report to build how often and who gets it. No
-- recipients means unconfigured: the report is still generated and shown,
-- but nothing is delivered (reporting recipients are the owner's input).
CREATE TABLE report_schedules (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    name         TEXT     NOT NULL,
    cadence      TEXT     NOT NULL CHECK (cadence IN ('daily', 'weekly', 'monthly', 'quarterly')),
    enabled      INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    params       TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    recipients   TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(recipients)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);

-- A report is one occurrence: a schedule (or an ad hoc request) for one
-- period. The occurrence key keeps a repeated schedule tick from building
-- and delivering it twice. result is the ReportingResult snapshot with its
-- formula version and freshness, so what was sent can be shown again
-- exactly. delivery_status unconfigured means no recipients or no mail
-- transport; it is never "sent".
CREATE TABLE reports (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    schedule_id     INTEGER  REFERENCES report_schedules (id),
    occurrence_key  TEXT     NOT NULL,
    cadence         TEXT     NOT NULL,
    period_start    TEXT     NOT NULL,
    period_end      TEXT     NOT NULL,
    params          TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    formula_version TEXT     NOT NULL,
    result          TEXT     NOT NULL CHECK (json_valid(result)),
    status          TEXT     NOT NULL CHECK (status IN ('generated', 'failed')),
    error_safe      TEXT,
    delivery_status TEXT     NOT NULL CHECK (delivery_status IN ('unconfigured', 'pending', 'sent', 'failed')),
    delivery_detail TEXT,
    delivered_at    DATETIME,
    generated_at    DATETIME NOT NULL,
    requested_by    INTEGER  REFERENCES users (id),
    UNIQUE (workspace_id, occurrence_key)
);
CREATE INDEX reports_workspace_generated ON reports (workspace_id, generated_at);

-- Anomaly checks and their thresholds, one versioned document per
-- workspace. No row means no check fires; a check with a null threshold
-- doesn't fire either. Urgent thresholds are the owner's input.
CREATE TABLE alert_configs (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

-- An alert is an observation that a check fired for a subject (a campaign,
-- an account). The same check firing again for the same subject while the
-- alert is open adds an occurrence instead of a new alert. Alerts never
-- change an ad account; they may later carry a draft proposal (B3.4).
CREATE TABLE alerts (
    id              INTEGER  PRIMARY KEY,
    workspace_id    INTEGER  NOT NULL REFERENCES workspaces (id),
    check_name      TEXT     NOT NULL,
    subject_key     TEXT     NOT NULL,
    subject_label   TEXT     NOT NULL,
    severity        TEXT     NOT NULL CHECK (severity IN ('info', 'warning', 'urgent')),
    state           TEXT     NOT NULL CHECK (state IN ('open', 'acknowledged', 'resolved')),
    message         TEXT     NOT NULL,
    evidence        TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(evidence)),
    config_version  INTEGER  NOT NULL,
    occurrences     INTEGER  NOT NULL DEFAULT 1,
    first_seen_at   DATETIME NOT NULL,
    last_seen_at    DATETIME NOT NULL,
    acknowledged_by INTEGER  REFERENCES users (id),
    acknowledged_at DATETIME,
    resolved_at     DATETIME
);
CREATE INDEX alerts_open ON alerts (workspace_id, state, check_name, subject_key);

-- +goose Down
DROP TABLE alerts;
DROP TABLE alert_configs;
DROP TABLE reports;
DROP TABLE report_schedules;
