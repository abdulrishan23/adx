-- name: UpsertAccountCapability :exec
INSERT INTO account_capabilities (account_id, capability, state, verified_at, checked_at, last_error)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, capability) DO UPDATE SET
    state = excluded.state,
    verified_at = COALESCE(excluded.verified_at, account_capabilities.verified_at),
    checked_at = excluded.checked_at,
    last_error = excluded.last_error;

-- name: ListAccountCapabilities :many
SELECT * FROM account_capabilities ORDER BY account_id, capability;

-- name: GetAccountCapability :one
SELECT * FROM account_capabilities WHERE account_id = ? AND capability = ?;

-- name: ListAllAccounts :many
SELECT sqlc.embed(accounts), sqlc.embed(credentials)
FROM accounts
JOIN credentials ON credentials.id = accounts.credential_id
ORDER BY accounts.type, accounts.external_id;

-- name: GetAccountByID :one
SELECT sqlc.embed(accounts), sqlc.embed(credentials)
FROM accounts
JOIN credentials ON credentials.id = accounts.credential_id
WHERE accounts.id = ?;

-- name: GetCurrentPolicy :one
SELECT * FROM policies WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: GetPolicyVersion :one
SELECT * FROM policies WHERE workspace_id = ? AND version = ?;

-- name: CreatePolicy :one
INSERT INTO policies (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListKillSwitches :many
SELECT * FROM kill_switches WHERE workspace_id = ? ORDER BY account_id;

-- name: UpsertKillSwitch :exec
INSERT INTO kill_switches (workspace_id, account_id, engaged, reason, set_by, set_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (workspace_id, account_id) DO UPDATE SET
    engaged = excluded.engaged, reason = excluded.reason, set_by = excluded.set_by, set_at = excluded.set_at;
