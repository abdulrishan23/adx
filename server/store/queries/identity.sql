-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = ?;

-- name: CreateMembership :exec
INSERT INTO memberships (workspace_id, user_id, role, enabled, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetMembership :one
SELECT * FROM memberships WHERE workspace_id = ? AND user_id = ?;

-- name: SetMembershipRole :exec
UPDATE memberships SET role = ? WHERE workspace_id = ? AND user_id = ?;

-- name: SetMembershipEnabled :exec
UPDATE memberships SET enabled = ? WHERE workspace_id = ? AND user_id = ?;

-- name: CreateSession :exec
INSERT INTO sessions (id_hash, user_id, workspace_id, via, created_at, last_seen_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetLiveSession :one
SELECT * FROM sessions WHERE id_hash = ? AND ended_at IS NULL;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = ? WHERE id_hash = ? AND ended_at IS NULL;

-- name: EndSession :execrows
UPDATE sessions SET ended_at = ? WHERE id_hash = ? AND ended_at IS NULL;

-- name: EndUserSessions :exec
UPDATE sessions SET ended_at = ? WHERE user_id = ? AND ended_at IS NULL;

-- name: CreateSignInAttempt :exec
INSERT INTO sign_in_attempts (state_hash, nonce_hash, code_verifier, return_to, expires_at)
VALUES (?, ?, ?, ?, ?);

-- name: ConsumeSignInAttempt :one
DELETE FROM sign_in_attempts WHERE state_hash = ? AND expires_at > ?
RETURNING *;

-- name: DeleteExpiredSignInAttempts :exec
DELETE FROM sign_in_attempts WHERE expires_at <= ?;
