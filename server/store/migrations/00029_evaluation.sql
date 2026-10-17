-- +goose Up
-- A qualification label is a person's agreed reading of a call, recorded
-- apart from the model's and from corrections, so accuracy is measured
-- against what the team says happened, not against what ADX wrote.
CREATE TABLE qualification_labels (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    call_id      INTEGER  NOT NULL UNIQUE REFERENCES calls (id),
    outcome      TEXT     NOT NULL,
    answers      TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(answers)),
    note         TEXT     NOT NULL DEFAULT '',
    labeled_by   INTEGER  REFERENCES users (id),
    labeled_at   DATETIME NOT NULL
);

-- +goose Down
DROP TABLE qualification_labels;
