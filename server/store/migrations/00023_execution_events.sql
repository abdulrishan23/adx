-- +goose Up
-- Every step an execution takes, for the timeline: who moved it, from what
-- to what, and the safe detail. Attempts are counted here as well as on the
-- intent so a retry is never silent.
CREATE TABLE execution_events (
    id         INTEGER  PRIMARY KEY,
    intent_id  INTEGER  NOT NULL REFERENCES execution_intents (id),
    attempt    INTEGER  NOT NULL,
    from_state TEXT     NOT NULL,
    to_state   TEXT     NOT NULL,
    detail     TEXT     NOT NULL DEFAULT '',
    by_user    INTEGER  REFERENCES users (id),
    at         DATETIME NOT NULL
);
CREATE INDEX execution_events_intent ON execution_events (intent_id, id);

-- +goose Down
DROP TABLE execution_events;
