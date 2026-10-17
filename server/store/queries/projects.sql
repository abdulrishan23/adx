-- name: ListProjects :many
SELECT * FROM projects
WHERE workspace_id = ?
ORDER BY name;

-- name: ListAllProjects :many
SELECT * FROM projects ORDER BY workspace_id, name;

-- name: CreateProject :one
INSERT INTO projects (workspace_id, name, created_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetProjectByName :one
SELECT * FROM projects WHERE workspace_id = ? AND name = ? COLLATE NOCASE;

-- name: GetProject :one
SELECT * FROM projects WHERE id = ? AND workspace_id = ?;
