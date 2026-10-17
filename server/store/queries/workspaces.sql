-- name: CreateWorkspace :one
INSERT INTO workspaces (slug, name, created_at) VALUES (?, ?, ?) RETURNING *;

-- name: ListWorkspaces :many
SELECT * FROM workspaces ORDER BY id;

-- name: GetWorkspaceBySlug :one
SELECT * FROM workspaces WHERE slug = ?;

-- name: ListMembershipsForUser :many
SELECT m.*, w.slug AS workspace_slug, w.name AS workspace_name FROM memberships m JOIN workspaces w ON w.id = m.workspace_id
WHERE m.user_id = ? AND m.enabled = 1 ORDER BY w.id;

-- name: ListMembershipsForWorkspace :many
SELECT m.*, u.email AS email, u.name AS user_name FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = ? ORDER BY m.id;

-- name: GetCurrentIndustryConfig :one
SELECT * FROM industry_configs WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: GetIndustryConfigVersion :one
SELECT * FROM industry_configs WHERE workspace_id = ? AND version = ?;

-- name: CreateIndustryConfig :one
INSERT INTO industry_configs (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListIndustryConfigs :many
SELECT * FROM industry_configs WHERE workspace_id = ? ORDER BY version DESC;
