-- +goose Up
CREATE TABLE connection_oauth_attempts (
    state_hash   TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,
    user_id      INTEGER NOT NULL,
    binding_hash TEXT NOT NULL,
    expires_at   DATETIME NOT NULL
);
CREATE INDEX connection_oauth_expiry ON connection_oauth_attempts(expires_at);

CREATE TABLE connections (
    provider         TEXT NOT NULL,
    location_id      TEXT NOT NULL,
    company_id       TEXT NOT NULL,
    name             TEXT NOT NULL,
    encrypted_tokens BLOB NOT NULL,
    scope            TEXT NOT NULL,
    token_expires_at DATETIME NOT NULL,
    connected_at     DATETIME NOT NULL,
    verified_at      DATETIME NOT NULL,
    PRIMARY KEY (provider, location_id)
);

-- +goose Down
DROP TABLE connections;
DROP TABLE connection_oauth_attempts;
