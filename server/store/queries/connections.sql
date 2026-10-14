-- name: CreateConnectionOAuthAttempt :exec
INSERT INTO connection_oauth_attempts (state_hash, provider, user_id, binding_hash, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: ConsumeConnectionOAuthAttempt :one
DELETE FROM connection_oauth_attempts
WHERE state_hash = ? AND provider = ? AND user_id = ? AND binding_hash = ? AND expires_at > ?
RETURNING state_hash;

-- name: DeleteExpiredConnectionOAuthAttempts :exec
DELETE FROM connection_oauth_attempts WHERE expires_at <= ?;

-- name: UpsertConnection :one
INSERT INTO connections
    (provider, location_id, company_id, name, encrypted_tokens, scope, token_expires_at, connected_at, verified_at, auth_method)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, location_id) DO UPDATE SET
    company_id = excluded.company_id,
    name = excluded.name,
    encrypted_tokens = excluded.encrypted_tokens,
    scope = excluded.scope,
    token_expires_at = excluded.token_expires_at,
    auth_method = excluded.auth_method,
    verified_at = excluded.verified_at
RETURNING *;

-- name: GetConnection :one
SELECT * FROM connections WHERE provider = ? AND location_id = ?;

-- name: ListConnections :many
SELECT * FROM connections WHERE provider = ? ORDER BY location_id;

-- name: UpdateConnectionCredentials :execrows
UPDATE connections
SET encrypted_tokens = ?, scope = ?, token_expires_at = ?
WHERE provider = ? AND location_id = ? AND encrypted_tokens = ?;

-- name: UpdateConnectionVerification :execrows
UPDATE connections SET name = ?, verified_at = ?
WHERE provider = ? AND location_id = ? AND encrypted_tokens = ?;
