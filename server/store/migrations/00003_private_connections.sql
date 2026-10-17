-- +goose Up
CREATE TABLE connections_new (
    provider TEXT NOT NULL,
    location_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    name TEXT NOT NULL,
    encrypted_tokens BLOB NOT NULL,
    scope TEXT NOT NULL,
    token_expires_at DATETIME,
    connected_at DATETIME NOT NULL,
    verified_at DATETIME NOT NULL,
    auth_method TEXT NOT NULL DEFAULT 'oauth' CHECK (auth_method IN ('oauth', 'private_token')),
    PRIMARY KEY (provider, location_id),
    CHECK ((auth_method = 'oauth' AND token_expires_at IS NOT NULL) OR
           (auth_method = 'private_token' AND token_expires_at IS NULL))
);
INSERT INTO connections_new
SELECT *, 'oauth' FROM connections;
DROP TABLE connections;
ALTER TABLE connections_new RENAME TO connections;

-- +goose Down
CREATE TABLE connections_old (
    provider TEXT NOT NULL,
    location_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    name TEXT NOT NULL,
    encrypted_tokens BLOB NOT NULL,
    scope TEXT NOT NULL,
    token_expires_at DATETIME NOT NULL,
    connected_at DATETIME NOT NULL,
    verified_at DATETIME NOT NULL,
    PRIMARY KEY (provider, location_id)
);
INSERT INTO connections_old
SELECT provider, location_id, company_id, name, encrypted_tokens, scope,
       token_expires_at, connected_at, verified_at
FROM connections WHERE auth_method = 'oauth';
DROP TABLE connections;
ALTER TABLE connections_old RENAME TO connections;
