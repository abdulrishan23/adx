
-- name: GetEvidenceByID :one
SELECT * FROM evidence WHERE id = ? AND workspace_id = ?;
