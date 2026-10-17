-- +goose Up
-- A research schedule re-runs a brief on a cadence and then writes a
-- comparative report for the period: our account against the previous
-- period, competitor pages against their last read, keyword estimates
-- against the last run, and recorded market events. The report proposes
-- hypotheses that cite evidence and never claim causation; people review
-- them, and the next report carries those reviews forward.
CREATE TABLE research_schedules (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    brief_id     INTEGER  NOT NULL REFERENCES research_briefs (id),
    name         TEXT     NOT NULL,
    cadence      TEXT     NOT NULL CHECK (cadence IN ('weekly', 'monthly', 'quarterly')),
    enabled      INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);
CREATE INDEX research_schedules_brief ON research_schedules (brief_id);

CREATE TABLE research_reports (
    id                  INTEGER  PRIMARY KEY,
    workspace_id        INTEGER  NOT NULL REFERENCES workspaces (id),
    brief_id            INTEGER  NOT NULL REFERENCES research_briefs (id),
    schedule_id         INTEGER  REFERENCES research_schedules (id),
    occurrence_key      TEXT     NOT NULL UNIQUE,
    period_start        DATE     NOT NULL,
    period_end          DATE     NOT NULL,
    previous_report_id  INTEGER  REFERENCES research_reports (id),
    limited             INTEGER  NOT NULL DEFAULT 0 CHECK (limited IN (0, 1)),
    limited_reason      TEXT     NOT NULL DEFAULT '',
    document            TEXT     NOT NULL CHECK (json_valid(document)),
    generated_at        DATETIME NOT NULL
);
CREATE INDEX research_reports_brief ON research_reports (brief_id, period_start);

-- One operator judgement per hypothesis per report. Accepting a hypothesis
-- records that the team believes it; the hypothesis itself stays a
-- hypothesis.
CREATE TABLE research_hypothesis_reviews (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    report_id      INTEGER  NOT NULL REFERENCES research_reports (id),
    hypothesis_key TEXT     NOT NULL,
    status         TEXT     NOT NULL CHECK (status IN ('accepted', 'rejected', 'corrected')),
    note           TEXT     NOT NULL DEFAULT '',
    decided_by     INTEGER  NOT NULL REFERENCES users (id),
    decided_at     DATETIME NOT NULL,
    UNIQUE (report_id, hypothesis_key)
);

-- +goose Down
DROP TABLE research_hypothesis_reviews;
DROP TABLE research_reports;
DROP TABLE research_schedules;
