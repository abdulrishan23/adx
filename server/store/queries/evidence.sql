
-- name: GetEvidenceByID :one
SELECT * FROM evidence WHERE id = ? AND workspace_id = ?;

-- name: CountEvidenceSince :one
SELECT count(*) FROM evidence WHERE workspace_id = ? AND source = ? AND kind = ? AND observed_at >= ?;
