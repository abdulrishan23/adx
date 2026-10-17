-- name: CreatePlan :one
INSERT INTO plans (workspace_id, brand_id, project_id, title, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPlan :one
SELECT * FROM plans WHERE id = ? AND workspace_id = ?;

-- name: ListPlans :many
SELECT * FROM plans WHERE workspace_id = ? ORDER BY updated_at DESC, id DESC LIMIT ?;

-- name: TouchPlan :exec
UPDATE plans SET updated_at = ? WHERE id = ?;

-- name: CreatePlanVersion :one
INSERT INTO plan_versions (plan_id, version, document, state, validation, source, note, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPlanVersion :one
SELECT * FROM plan_versions WHERE plan_id = ? AND version = ?;

-- name: GetLatestPlanVersion :one
SELECT * FROM plan_versions WHERE plan_id = ? ORDER BY version DESC LIMIT 1;

-- name: ListPlanVersions :many
SELECT * FROM plan_versions WHERE plan_id = ? ORDER BY version;

-- name: AcceptPlanVersion :exec
UPDATE plan_versions SET state = 'accepted', accepted_by = ?, accepted_at = ? WHERE id = ? AND state = 'ready';
