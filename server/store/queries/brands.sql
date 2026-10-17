-- name: CreateBrand :one
INSERT INTO brands (workspace_id, name, kind, notes, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBrand :one
SELECT * FROM brands WHERE id = ? AND workspace_id = ?;

-- name: ListBrands :many
SELECT * FROM brands WHERE workspace_id = ? ORDER BY kind, name;

-- name: ListProjectsByBrand :many
SELECT * FROM projects WHERE brand_id = ? ORDER BY name;

-- name: ListUnassignedProjects :many
SELECT * FROM projects WHERE brand_id IS NULL ORDER BY name;

-- name: SetProjectBrand :execrows
UPDATE projects SET brand_id = ? WHERE id = ?;

-- name: CreateOffer :one
INSERT INTO offers (workspace_id, brand_id, project_id, name, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOffer :one
SELECT * FROM offers WHERE id = ? AND workspace_id = ?;

-- name: ListOffersByBrand :many
SELECT * FROM offers WHERE workspace_id = ? AND brand_id = ? ORDER BY name, id;

-- name: CreateOfferVersion :one
INSERT INTO offer_versions (offer_id, version, document, effective_from, effective_to, state, note, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCurrentOfferVersion :one
SELECT * FROM offer_versions WHERE offer_id = ? ORDER BY version DESC LIMIT 1;

-- name: GetOfferVersion :one
SELECT * FROM offer_versions WHERE offer_id = ? AND version = ?;

-- name: ListOfferVersions :many
SELECT * FROM offer_versions WHERE offer_id = ? ORDER BY version;

-- name: SetOfferVersionState :exec
UPDATE offer_versions SET state = ?, approved_by = ?, approved_at = ? WHERE id = ?;

-- name: ListCreativesByOffer :many
SELECT c.*, v.version AS current_version, v.review_state
FROM creatives c
JOIN creative_versions v ON v.creative_id = c.id
 AND v.version = (SELECT MAX(version) FROM creative_versions WHERE creative_id = c.id)
WHERE c.workspace_id = ? AND c.offer_id = ?
ORDER BY c.id;

-- name: CreateFact :one
INSERT INTO facts (workspace_id, brand_id, project_id, key, kind, value, currency, effective_from, effective_to, class, source_kind, source_ref, evidence_id, note, created_by, created_at, approved_by, approved_at, supersedes_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetFact :one
SELECT * FROM facts WHERE id = ? AND workspace_id = ?;

-- name: ListCurrentFacts :many
SELECT * FROM facts
WHERE workspace_id = sqlc.arg(workspace_id) AND brand_id = sqlc.arg(brand_id)
  AND class != 'retired'
  AND (sqlc.narg(project_id) IS NULL OR project_id IS NULL OR project_id = sqlc.narg(project_id))
ORDER BY key, effective_from, id;

-- name: ListFactHistory :many
SELECT * FROM facts
WHERE workspace_id = ? AND brand_id = ? AND key = ?
ORDER BY id;

-- name: ListCurrentFactsByKey :many
SELECT * FROM facts
WHERE workspace_id = ? AND brand_id = ? AND key = ? AND class != 'retired'
ORDER BY id;

-- name: ListClaims :many
SELECT * FROM facts
WHERE workspace_id = sqlc.arg(workspace_id) AND brand_id = sqlc.arg(brand_id)
  AND class = 'approved'
  AND (project_id IS NULL OR project_id = sqlc.narg(project_id))
  AND effective_from <= sqlc.arg(at)
  AND (effective_to IS NULL OR effective_to >= sqlc.arg(at))
ORDER BY key, project_id DESC, effective_from DESC, id DESC;

-- name: ApproveFact :exec
UPDATE facts SET class = 'approved', approved_by = ?, approved_at = ? WHERE id = ?;

-- name: RetireFact :exec
UPDATE facts SET class = 'retired', retired_at = ?, retired_reason = ? WHERE id = ?;

-- name: CreateFactConflict :one
INSERT INTO fact_conflicts (workspace_id, brand_id, key, fact_a_id, fact_b_id, state, opened_at)
VALUES (?, ?, ?, ?, ?, 'open', ?)
RETURNING *;

-- name: GetFactConflict :one
SELECT * FROM fact_conflicts WHERE id = ? AND workspace_id = ?;

-- name: ListOpenConflicts :many
SELECT * FROM fact_conflicts WHERE workspace_id = ? AND brand_id = ? AND state = 'open' ORDER BY id;

-- name: ListOpenConflictsForFact :many
SELECT * FROM fact_conflicts
WHERE state = 'open' AND (fact_a_id = sqlc.arg(fact_id) OR fact_b_id = sqlc.arg(fact_id))
ORDER BY id;

-- name: ResolveFactConflict :exec
UPDATE fact_conflicts SET state = 'resolved', resolved_by = ?, resolved_at = ?, winner_id = ?, resolution = ? WHERE id = ?;
