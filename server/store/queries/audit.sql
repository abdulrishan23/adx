-- name: CreateAuditEntry :exec
INSERT INTO audit_log (at, actor_user_id, via, action, target_user_id, detail)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListAuditEntries :many
SELECT * FROM audit_log
ORDER BY id;
