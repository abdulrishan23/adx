
-- name: GetEvidenceByID :one
SELECT * FROM evidence WHERE id = ? AND workspace_id = ?;

-- name: CountEvidenceSince :one
SELECT count(*) FROM evidence WHERE workspace_id = ? AND source = ? AND kind = ? AND observed_at >= ?;

-- name: ListEvidenceVersions :many
SELECT * FROM evidence WHERE workspace_id = ? AND source = ? AND kind = ? AND source_ref = ? ORDER BY payload_version DESC LIMIT ?;

-- name: ListCurrentEvidenceByScope :many
SELECT * FROM evidence WHERE workspace_id = ? AND source = ? AND kind = ? AND scope = ? AND superseded_by IS NULL ORDER BY id;

-- name: ListCurrentEvidenceByKind :many
SELECT * FROM evidence WHERE workspace_id = ? AND source = ? AND kind = ? AND superseded_by IS NULL ORDER BY id;
