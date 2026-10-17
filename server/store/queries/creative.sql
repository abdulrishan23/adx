-- name: CreateAsset :one
INSERT INTO assets (workspace_id, project_id, kind, name, key, sha256, size, content_type, uploaded_by, created_at, provenance)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAsset :one
SELECT * FROM assets WHERE id = ? AND workspace_id = ?;

-- name: GetAssetByKey :one
SELECT * FROM assets WHERE workspace_id = ? AND key = ? ORDER BY id LIMIT 1;

-- name: ListAssets :many
SELECT * FROM assets
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(project_id) IS NULL OR project_id = sqlc.narg(project_id))
  AND (sqlc.narg(kind) IS NULL OR kind = sqlc.narg(kind))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: GetCurrentBrandKit :one
SELECT * FROM brand_kits WHERE project_id = ? AND workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreateBrandKit :one
INSERT INTO brand_kits (workspace_id, project_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateCreative :one
INSERT INTO creatives (workspace_id, project_id, name, format, language, offer_id, offer_version, plan_id, plan_version, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCreative :one
SELECT * FROM creatives WHERE id = ? AND workspace_id = ?;

-- name: TouchCreative :exec
UPDATE creatives SET updated_at = ? WHERE id = ?;

-- name: ListCreatives :many
SELECT * FROM creatives
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(project_id) IS NULL OR project_id = sqlc.narg(project_id))
ORDER BY updated_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: CreateCreativeVersion :one
INSERT INTO creative_versions (creative_id, version, document, review_state, created_by, created_at)
VALUES (?, ?, ?, 'draft', ?, ?)
RETURNING *;

-- name: GetCreativeVersion :one
SELECT * FROM creative_versions WHERE id = ?;

-- name: GetLatestCreativeVersion :one
SELECT * FROM creative_versions WHERE creative_id = ? ORDER BY version DESC LIMIT 1;

-- name: ListCreativeVersions :many
SELECT * FROM creative_versions WHERE creative_id = ? ORDER BY version;

-- name: UpdateCreativeDraft :execrows
UPDATE creative_versions SET document = ? WHERE id = ? AND review_state = 'draft';

-- name: SetCreativeVersionRendered :execrows
UPDATE creative_versions
SET review_state = 'rendered', renderer = ?, output_key = ?, output_sha256 = ?, output_size = ?, output_type = ?, rendered_at = ?, bindings = ?, render_run_id = ?, render_error = ''
WHERE id = ? AND review_state IN ('draft', 'rendered');

-- name: SetCreativeVersionRenderError :exec
UPDATE creative_versions SET render_error = ?, render_run_id = ? WHERE id = ?;

-- name: ListCreativesByPlan :many
SELECT * FROM creatives WHERE workspace_id = ? AND plan_id = ? ORDER BY id;

-- name: ListCreativesByState :many
SELECT c.*, v.id AS version_id, v.version AS current_version, v.review_state, v.render_error
FROM creatives c
JOIN creative_versions v ON v.creative_id = c.id
 AND v.version = (SELECT MAX(version) FROM creative_versions WHERE creative_id = c.id)
WHERE c.workspace_id = ?
ORDER BY c.updated_at DESC, c.id DESC;

-- name: SetCreativeVersionState :execrows
UPDATE creative_versions SET review_state = sqlc.arg(to_state) WHERE id = sqlc.arg(id) AND review_state = sqlc.arg(from_state);

-- name: CreateCreativeReview :one
INSERT INTO creative_reviews (version_id, decision, output_sha256, note, decided_by, decided_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListCreativeReviews :many
SELECT * FROM creative_reviews WHERE version_id = ? ORDER BY decided_at, id;
