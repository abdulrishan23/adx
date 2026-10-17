-- +goose Up
-- A call session is what the conversation produced: the transcript as
-- evidence, the structured answers, the summary and how far the model
-- trusted its own reading. review_status follows the reference source:
-- auto (applied as read), pending (low confidence, a person decides),
-- confirmed, corrected. applied_key is the occurrence key the outcome was
-- applied under, so a repeated completion applies nothing twice.
CREATE TABLE call_sessions (
    id                     INTEGER  PRIMARY KEY,
    workspace_id           INTEGER  NOT NULL REFERENCES workspaces (id),
    call_id                INTEGER  NOT NULL UNIQUE REFERENCES calls (id),
    speech_provider        TEXT     NOT NULL,
    state                  TEXT     NOT NULL CHECK (state IN ('running', 'ended', 'failed')),
    ended_by               TEXT     NOT NULL DEFAULT '',
    error                  TEXT     NOT NULL DEFAULT '',
    interruptions          INTEGER  NOT NULL DEFAULT 0,
    frames_dropped         INTEGER  NOT NULL DEFAULT 0,
    transcript             TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(transcript)),
    transcript_evidence_id INTEGER  REFERENCES evidence (id),
    summary                TEXT     NOT NULL DEFAULT '',
    answers                TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(answers)),
    outcome                TEXT     NOT NULL DEFAULT '',
    confidence             TEXT     NOT NULL DEFAULT '' CHECK (confidence IN ('', 'high', 'low')),
    requested_human        INTEGER  NOT NULL DEFAULT 0,
    callback_at            TEXT     NOT NULL DEFAULT '',
    review_status          TEXT     NOT NULL DEFAULT 'pending' CHECK (review_status IN ('auto', 'pending', 'confirmed', 'corrected')),
    review_reason          TEXT     NOT NULL DEFAULT '',
    applied_key            TEXT     NOT NULL DEFAULT '',
    applied_version        INTEGER  NOT NULL DEFAULT 0,
    applied_at             DATETIME,
    applied_detail         TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(applied_detail)),
    started_at             DATETIME NOT NULL,
    ended_at               DATETIME,
    updated_at             DATETIME NOT NULL
);

CREATE TABLE call_corrections (
    id         INTEGER  PRIMARY KEY,
    session_id INTEGER  NOT NULL REFERENCES call_sessions (id),
    outcome    TEXT     NOT NULL,
    answers    TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(answers)),
    note       TEXT     NOT NULL,
    created_by INTEGER  REFERENCES users (id),
    created_at DATETIME NOT NULL
);
CREATE INDEX call_corrections_session ON call_corrections (session_id);

-- +goose Down
DROP TABLE call_corrections;
DROP TABLE call_sessions;
