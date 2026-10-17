-- name: ListProjects :many
SELECT * FROM projects
ORDER BY name;

-- name: CreateProject :one
INSERT INTO projects (name, created_at)
VALUES (?, ?)
RETURNING *;
