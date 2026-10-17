-- +goose Up
-- Ask ADX conversations. A conversation belongs to the person who started it
-- and runs with their role: the assistant can read only what they can read.
CREATE TABLE assistant_conversations (
    id           INTEGER  PRIMARY KEY,
    workspace_id INTEGER  NOT NULL REFERENCES workspaces (id),
    user_id      INTEGER  REFERENCES users (id),
    title        TEXT     NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL,
    updated_at   DATETIME NOT NULL
);
CREATE INDEX assistant_conversations_user ON assistant_conversations (workspace_id, user_id, updated_at);

-- One row per turn. role is user, assistant or tool. For assistant turns,
-- content is the text, tool_calls the calls it asked for, and status says
-- whether the model answered (complete), declined (refused) or was
-- unavailable (nothing invented, retryable). Tool turns hold the tool's
-- name, input and the evidence references its result carried. Tokens and
-- cost are recorded per turn; cost is null when no price is configured.
CREATE TABLE assistant_messages (
    id              INTEGER  PRIMARY KEY,
    conversation_id INTEGER  NOT NULL REFERENCES assistant_conversations (id),
    role            TEXT     NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
    status          TEXT     NOT NULL DEFAULT 'complete' CHECK (status IN ('complete', 'unavailable', 'refused')),
    content         TEXT     NOT NULL DEFAULT '',
    tool_calls      TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(tool_calls)),
    tool_name       TEXT,
    tool_input      TEXT     CHECK (tool_input IS NULL OR json_valid(tool_input)),
    tool_result     TEXT     CHECK (tool_result IS NULL OR json_valid(tool_result)),
    evidence_refs   TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(evidence_refs)),
    warnings        TEXT     NOT NULL DEFAULT '[]' CHECK (json_valid(warnings)),
    model           TEXT,
    input_tokens    INTEGER,
    output_tokens   INTEGER,
    cost_micros     INTEGER,
    created_at      DATETIME NOT NULL
);
CREATE INDEX assistant_messages_conversation ON assistant_messages (conversation_id, id);

-- +goose Down
DROP TABLE assistant_messages;
DROP TABLE assistant_conversations;
