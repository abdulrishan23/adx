-- name: CreateResearchBrief :one
INSERT INTO research_briefs (workspace_id, brand_id, project_id, title, question, geography, language, google_ads_account_id, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetResearchBrief :one
SELECT * FROM research_briefs WHERE id = ? AND workspace_id = ?;

-- name: ListResearchBriefs :many
SELECT * FROM research_briefs WHERE workspace_id = ? ORDER BY created_at DESC, id DESC LIMIT ?;

-- name: CreateResearchItem :one
INSERT INTO research_items (workspace_id, brief_id, kind, target, competitor_name, competitor_brand_id, identity_state, candidates, state, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListResearchItems :many
SELECT * FROM research_items WHERE brief_id = ? ORDER BY id;

-- name: GetResearchItem :one
SELECT * FROM research_items WHERE id = ? AND workspace_id = ?;

-- name: ResolveResearchItemsCompetitor :execrows
UPDATE research_items
SET competitor_brand_id = ?, identity_state = 'resolved', candidates = '[]', state = 'pending'
WHERE brief_id = ? AND competitor_name = ? COLLATE NOCASE AND state = 'needs_identity';

-- name: MarkResearchItemFetched :exec
UPDATE research_items SET state = 'fetched', error_safe = '', attempts = attempts + 1, fetched_at = ?, run_id = ?, evidence_id = ? WHERE id = ?;

-- name: MarkResearchItemFailed :exec
UPDATE research_items SET state = ?, error_safe = ?, attempts = attempts + 1, run_id = ? WHERE id = ?;

-- name: DeleteResearchFindingsForItem :exec
DELETE FROM research_findings WHERE item_id = ?;

-- name: CreateResearchFinding :one
INSERT INTO research_findings (workspace_id, brief_id, item_id, scope, subject_brand_id, kind, class, label, payload, source_url, source_observed_at, geography, evidence_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListResearchFindings :many
SELECT * FROM research_findings
WHERE brief_id = sqlc.arg(brief_id)
  AND (sqlc.narg(scope) IS NULL OR scope = sqlc.narg(scope))
ORDER BY scope, kind, id;

-- name: CountResearchRunsForBrief :one
SELECT COUNT(DISTINCT run_id) FROM research_items WHERE brief_id = ? AND run_id IS NOT NULL;

-- name: ListCompetitorBrands :many
SELECT * FROM brands WHERE workspace_id = ? AND kind = 'competitor' ORDER BY name;
