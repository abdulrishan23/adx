-- +goose Up
-- Interaction events are the signals ADX observes about a person: a form,
-- a page view tied to a person, a CRM stage change, a webhook from a
-- supported owned channel. occurrence_key makes a repeated delivery the same
-- event. confidence says how sure the identity is: an exact identifier match
-- is confirmed; a device or anonymous id alone is unknown and never scores.
CREATE TABLE interaction_events (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id      INTEGER  REFERENCES people (id),
    enquiry_id     INTEGER  REFERENCES enquiries (id),
    source         TEXT     NOT NULL,
    kind           TEXT     NOT NULL,
    occurrence_key TEXT     NOT NULL,
    confidence     TEXT     NOT NULL CHECK (confidence IN ('confirmed', 'unknown')),
    identity       TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(identity)),
    payload        TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
    occurred_at    DATETIME NOT NULL,
    received_at    DATETIME NOT NULL,
    evidence_id    INTEGER  REFERENCES evidence (id),
    discarded_at   DATETIME,
    discard_reason TEXT     NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX interaction_events_occurrence ON interaction_events (workspace_id, source, occurrence_key);
CREATE INDEX interaction_events_person ON interaction_events (person_id, occurred_at);

-- Scoring rules are versioned. An assessment records the version it used,
-- so a rule change never rewrites history.
CREATE TABLE scoring_rules (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

CREATE TABLE score_assessments (
    id            INTEGER  PRIMARY KEY,
    workspace_id  INTEGER  NOT NULL REFERENCES workspaces (id),
    person_id     INTEGER  NOT NULL REFERENCES people (id),
    rules_version INTEGER,
    trigger       TEXT     NOT NULL,
    fit           INTEGER,
    intent        INTEGER,
    temperature   TEXT,
    explanation   TEXT     NOT NULL CHECK (json_valid(explanation)),
    assessed_at   DATETIME NOT NULL
);
CREATE INDEX score_assessments_person ON score_assessments (person_id, assessed_at);

CREATE TABLE signal_corrections (
    id             INTEGER  PRIMARY KEY,
    workspace_id   INTEGER  NOT NULL REFERENCES workspaces (id),
    event_id       INTEGER  NOT NULL REFERENCES interaction_events (id),
    action         TEXT     NOT NULL CHECK (action IN ('discard', 'reattribute')),
    from_person_id INTEGER  REFERENCES people (id),
    to_person_id   INTEGER  REFERENCES people (id),
    reason         TEXT     NOT NULL,
    created_by     INTEGER  REFERENCES users (id),
    created_at     DATETIME NOT NULL
);
CREATE INDEX signal_corrections_event ON signal_corrections (event_id);

-- +goose Down
DROP TABLE signal_corrections;
DROP TABLE score_assessments;
DROP TABLE scoring_rules;
DROP TABLE interaction_events;
