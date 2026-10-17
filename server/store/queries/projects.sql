-- name: ListProjects :many
SELECT * FROM projects
ORDER BY name;

-- name: CreateProject :one
INSERT INTO projects (name, created_at)
VALUES (?, ?)
RETURNING *;

-- name: GetProjectByName :one
SELECT * FROM projects WHERE name = ? COLLATE NOCASE;

-- name: GetProject :one
SELECT * FROM projects WHERE id = ?;
