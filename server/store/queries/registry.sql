-- name: CreateRegistryEntry :one
INSERT INTO registry_entries (workspace_id, kind, key, version, state, source, document, validation, needs_admin, reason, created_by, created_at)
VALUES (?, ?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetRegistryEntry :one
SELECT * FROM registry_entries WHERE id = ? AND workspace_id = ?;

-- name: GetRegistryVersion :one
SELECT * FROM registry_entries WHERE workspace_id = ? AND kind = ? AND key = ? AND version = ?;

-- name: GetActiveRegistryEntry :one
SELECT * FROM registry_entries WHERE workspace_id = ? AND kind = ? AND key = ? AND state = 'active' ORDER BY version DESC LIMIT 1;

-- name: GetLatestRegistryVersion :one
SELECT * FROM registry_entries WHERE workspace_id = ? AND kind = ? AND key = ? ORDER BY version DESC LIMIT 1;

-- name: ListRegistryEntries :many
SELECT * FROM registry_entries WHERE workspace_id = ? AND kind = ? ORDER BY key, version DESC;

-- name: ListRegistryVersions :many
SELECT * FROM registry_entries WHERE workspace_id = ? AND kind = ? AND key = ? ORDER BY version DESC;

-- name: ActivateRegistryEntry :execrows
UPDATE registry_entries SET state = 'active', activated_by = ?, activated_at = ?, reason = ? WHERE id = ? AND state = 'draft';

-- name: RetireActiveRegistryEntries :exec
UPDATE registry_entries SET state = 'retired', retired_at = ? WHERE workspace_id = ? AND kind = ? AND key = ? AND state = 'active' AND id != ?;

-- name: RetireRegistryEntry :execrows
UPDATE registry_entries SET state = 'retired', retired_at = ? WHERE id = ? AND state IN ('draft', 'active');
