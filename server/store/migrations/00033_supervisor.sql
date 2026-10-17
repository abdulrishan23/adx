-- +goose Up
-- The supervisor: objective precedence and autonomy modes per capability
-- (versioned), recommendations (a proposal with where it came from and
-- which objective it serves), and conflicts between recommendations over
-- one budget envelope, arbitrated by precedence or escalated to a person.
CREATE TABLE supervisor_configs (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    version      INTEGER  NOT NULL,
    document     TEXT     NOT NULL CHECK (json_valid(document)),
    created_by   INTEGER  REFERENCES users (id),
    created_at   DATETIME NOT NULL,
    UNIQUE (workspace_id, version)
);

CREATE TABLE recommendations (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    proposal_id  INTEGER  NOT NULL UNIQUE REFERENCES proposals (id),
    source       TEXT     NOT NULL,
    capability   TEXT     NOT NULL,
    objective    TEXT     NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL
);
CREATE INDEX recommendations_day ON recommendations (workspace_id, capability, created_at);

CREATE TABLE supervisor_conflicts (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    envelope_id  INTEGER  NOT NULL REFERENCES budget_envelopes (id),
    proposal_ids TEXT     NOT NULL CHECK (json_valid(proposal_ids)),
    state        TEXT     NOT NULL CHECK (state IN ('arbitrated', 'escalated', 'resolved')),
    detail       TEXT     NOT NULL DEFAULT '',
    resolution   TEXT     NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL,
    resolved_by  INTEGER  REFERENCES users (id),
    resolved_at  DATETIME
);
CREATE INDEX supervisor_conflicts_state ON supervisor_conflicts (workspace_id, state);

-- +goose Down
DROP TABLE supervisor_conflicts;
DROP TABLE recommendations;
DROP TABLE supervisor_configs;
