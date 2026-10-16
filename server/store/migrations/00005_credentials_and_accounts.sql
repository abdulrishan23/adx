-- +goose Up
-- A credential is what lets ADX into a provider: a GoHighLevel private token,
-- an OAuth grant, or a service account. An account is something ADX reads
-- through a credential, such as a GoHighLevel location or a Google Ads
-- account. One Google or Meta sign-in reaches many accounts, so they're kept
-- apart, and each account has one row whichever credential reaches it.
CREATE TABLE credentials (
    id               INTEGER  PRIMARY KEY,
    provider         TEXT     NOT NULL,
    kind             TEXT     NOT NULL CHECK (kind IN ('oauth', 'private_token', 'service_account')),
    -- Who the provider says granted access: the location a GoHighLevel token
    -- belongs to, or a Google account's stable ID. Reconnecting as the same
    -- subject replaces the secret instead of adding a credential.
    subject          TEXT     NOT NULL,
    encrypted_secret BLOB     NOT NULL,
    -- The encryption key that sealed the secret, so the key can be rotated.
    key_version      INTEGER  NOT NULL,
    -- Space separated. A private token reports no scopes, so these are the
    -- ones its last check proved.
    scopes           TEXT     NOT NULL,
    token_expires_at DATETIME,
    -- Who saved the current secret, and when. Null when no person is known,
    -- such as for credentials saved before ADX recorded it.
    connected_by     INTEGER  REFERENCES users (id),
    connected_at     DATETIME NOT NULL,
    UNIQUE (provider, subject)
);

CREATE TABLE accounts (
    id            INTEGER  PRIMARY KEY,
    type          TEXT     NOT NULL,
    external_id   TEXT     NOT NULL,
    credential_id INTEGER  NOT NULL REFERENCES credentials (id),
    -- The Google Ads manager, GA4 account or GoHighLevel company above it.
    parent_id     TEXT,
    name          TEXT,
    currency      TEXT,
    -- The time zone the provider's days are in.
    time_zone     TEXT,
    -- Facts only this provider's code reads, such as a GoHighLevel country.
    details       TEXT     NOT NULL DEFAULT '{}' CHECK (json_valid(details)),
    connected_at  DATETIME NOT NULL,
    verified_at   DATETIME NOT NULL,
    UNIQUE (type, external_id)
);

CREATE INDEX accounts_credential ON accounts (credential_id);

-- Existing secrets stay valid because they were sealed with
-- "gohighlevel:<location>", which is "<provider>:<subject>" here.
INSERT INTO credentials
    (provider, kind, subject, encrypted_secret, key_version, scopes, token_expires_at, connected_at)
SELECT provider, auth_method, location_id, encrypted_tokens, 1, scope, token_expires_at, connected_at
FROM connections;

INSERT INTO accounts
    (type, external_id, credential_id, parent_id, name, connected_at, verified_at)
SELECT c.provider, c.location_id, cr.id, NULLIF(c.company_id, ''), c.name, c.connected_at, c.verified_at
FROM connections c
JOIN credentials cr ON cr.provider = c.provider AND cr.subject = c.location_id;

DROP TABLE connections;

-- +goose Down
CREATE TABLE connections (
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

INSERT INTO connections
    (provider, location_id, company_id, name, encrypted_tokens, scope, token_expires_at,
     connected_at, verified_at, auth_method)
SELECT a.type, a.external_id, COALESCE(a.parent_id, ''), COALESCE(a.name, a.external_id),
       cr.encrypted_secret, cr.scopes, cr.token_expires_at, a.connected_at, a.verified_at, cr.kind
FROM accounts a
JOIN credentials cr ON cr.id = a.credential_id
WHERE a.type = 'gohighlevel';

DROP TABLE accounts;
DROP TABLE credentials;
