-- name: ListUsers :many
SELECT * FROM users
ORDER BY email;

-- name: GetUser :one
SELECT * FROM users
WHERE id = ?;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = ?;

-- name: GetUserByAuthentikUUID :one
SELECT * FROM users
WHERE authentik_uuid = ?;

-- name: CreateUser :one
INSERT INTO users (email, role, status, authentik_pk, authentik_uuid, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: SetUserRole :exec
UPDATE users SET role = ?
WHERE id = ?;

-- name: SetUserStatus :exec
UPDATE users SET status = ?
WHERE id = ?;

-- name: RecordSignIn :one
UPDATE users SET status = 'active', name = ?, last_sign_in_at = ?
WHERE id = ?
RETURNING *;

-- name: CountEnabledAdmins :one
SELECT count(*) FROM users
WHERE role = 'admin' AND status != 'disabled';
