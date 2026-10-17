-- name: CreateConnectionOAuthAttempt :exec
INSERT INTO connection_oauth_attempts (state_hash, provider, user_id, binding_hash, expires_at, code_verifier)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ConsumeConnectionOAuthAttempt :one
DELETE FROM connection_oauth_attempts
WHERE state_hash = ? AND provider = ? AND user_id = ? AND binding_hash = ? AND expires_at > ?
RETURNING *;

-- name: DeleteExpiredConnectionOAuthAttempts :exec
DELETE FROM connection_oauth_attempts WHERE expires_at <= ?;

-- name: UpsertCredential :one
INSERT INTO credentials
    (provider, kind, subject, encrypted_secret, key_version, scopes, token_expires_at, connected_by, connected_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, subject) DO UPDATE SET
    kind = excluded.kind,
    encrypted_secret = excluded.encrypted_secret,
    key_version = excluded.key_version,
    scopes = excluded.scopes,
    token_expires_at = excluded.token_expires_at,
    connected_by = excluded.connected_by,
    connected_at = excluded.connected_at
RETURNING *;

-- name: UpsertAccount :one
INSERT INTO accounts
    (type, external_id, credential_id, parent_id, name, currency, time_zone, details, connected_at, verified_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (type, external_id) DO UPDATE SET
    credential_id = excluded.credential_id,
    parent_id = excluded.parent_id,
    name = excluded.name,
    currency = excluded.currency,
    time_zone = excluded.time_zone,
    details = excluded.details,
    verified_at = excluded.verified_at
RETURNING *;

-- name: GetAccount :one
SELECT sqlc.embed(accounts), sqlc.embed(credentials)
FROM accounts
JOIN credentials ON credentials.id = accounts.credential_id
WHERE accounts.type = ? AND accounts.external_id = ?;

-- name: ListAccounts :many
SELECT sqlc.embed(accounts), sqlc.embed(credentials)
FROM accounts
JOIN credentials ON credentials.id = accounts.credential_id
WHERE accounts.type = ?
ORDER BY accounts.external_id;

-- Updates only if nobody replaced the secret since it was read.
-- name: UpdateCredentialSecret :execrows
UPDATE credentials
SET encrypted_secret = sqlc.arg(new_secret), key_version = sqlc.arg(key_version),
    scopes = sqlc.arg(scopes), token_expires_at = sqlc.arg(token_expires_at)
WHERE id = sqlc.arg(id) AND encrypted_secret = sqlc.arg(old_secret);

-- Updates only if the account still uses the secret the check was made with.
-- name: UpdateAccountVerification :execrows
UPDATE accounts
SET name = ?, time_zone = ?, details = ?, verified_at = ?
WHERE accounts.id = ? AND EXISTS (
    SELECT 1 FROM credentials
    WHERE credentials.id = accounts.credential_id AND credentials.encrypted_secret = ?
);

-- name: ListAccountsByCredential :many
SELECT * FROM accounts WHERE credential_id = ? ORDER BY external_id;
