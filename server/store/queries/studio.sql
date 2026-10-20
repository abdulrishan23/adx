-- name: GetFile :one
SELECT * FROM files WHERE id = ?;

-- name: GetFileBySHA :one
SELECT * FROM files WHERE sha256 = ?;

-- name: InsertFile :one
INSERT INTO files (sha256, content_type, bytes, width, height, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListCreativeProjects :many
SELECT p.id AS project_id, p.name, c.prefix, c.review_active,
       (SELECT COUNT(*) FROM creative_concepts k WHERE k.project_id = c.project_id) AS concepts,
       (SELECT COUNT(*) FROM creative_ads a WHERE a.project_id = c.project_id) AS ads,
       (SELECT COUNT(*) FROM creative_renders r WHERE r.project_id = c.project_id AND r.hidden = 0) AS renders,
       c.brand_kit, c.updated_at
FROM creative_projects c
JOIN projects p ON p.id = c.project_id
WHERE p.workspace_id = ?
ORDER BY p.name COLLATE NOCASE;

-- name: GetCreativeProject :one
SELECT c.*, p.name AS project_name
FROM creative_projects c
JOIN projects p ON p.id = c.project_id
WHERE c.project_id = ?;

-- name: GetCreativeProjectByKey :one
SELECT c.*, p.name AS project_name
FROM creative_projects c
JOIN projects p ON p.id = c.project_id
WHERE c.review_key = ?;

-- name: InsertCreativeProject :exec
INSERT INTO creative_projects (project_id, prefix, kicker, page_name, review_key, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateCreativeSettings :exec
UPDATE creative_projects
SET prefix = ?, kicker = ?, page_name = ?, review_active = ?, updated_at = ?
WHERE project_id = ?;

-- name: SetReviewKey :exec
UPDATE creative_projects SET review_key = ?, updated_at = ? WHERE project_id = ?;

-- name: SaveBrandKit :exec
UPDATE creative_projects SET brand_kit = ?, updated_at = ? WHERE project_id = ?;

-- name: TouchCreativeProject :exec
UPDATE creative_projects SET updated_at = ? WHERE project_id = ?;

-- name: ListRenders :many
SELECT * FROM creative_renders WHERE project_id = ? ORDER BY pos, code;

-- name: GetRender :one
SELECT * FROM creative_renders WHERE id = ?;

-- name: GetRenderByCode :one
SELECT * FROM creative_renders WHERE project_id = ? AND code = ?;

-- name: InsertRender :one
INSERT INTO creative_renders (project_id, code, pos, label, file_id, preview_file_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateRender :exec
UPDATE creative_renders SET label = ?, hidden = ? WHERE id = ?;

-- name: ListConcepts :many
SELECT * FROM creative_concepts WHERE project_id = ? ORDER BY pos, code;

-- name: GetConcept :one
SELECT * FROM creative_concepts WHERE id = ?;

-- name: GetConceptByCode :one
SELECT * FROM creative_concepts WHERE project_id = ? AND code = ?;

-- name: MaxConceptPos :one
SELECT CAST(COALESCE(MAX(pos), 0) AS INTEGER) FROM creative_concepts WHERE project_id = ?;

-- name: InsertConcept :one
INSERT INTO creative_concepts (project_id, code, pos, name, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateConcept :exec
UPDATE creative_concepts
SET name = ?, why = ?, primary_text = ?, headline = ?, description = ?, cta = ?, copy = ?, updated_at = ?
WHERE id = ?;

-- name: SetConceptImagery :exec
UPDATE creative_concepts SET imagery = ?, updated_at = ? WHERE id = ?;

-- name: ListAds :many
SELECT * FROM creative_ads WHERE project_id = ? ORDER BY pos, code;

-- name: ListConceptAds :many
SELECT * FROM creative_ads WHERE concept_id = ? ORDER BY pos, code;

-- name: GetAd :one
SELECT * FROM creative_ads WHERE id = ?;

-- name: GetAdByCode :one
SELECT * FROM creative_ads WHERE project_id = ? AND code = ?;

-- name: MaxAdPos :one
SELECT CAST(COALESCE(MAX(pos), 0) AS INTEGER) FROM creative_ads WHERE concept_id = ?;

-- name: InsertAd :one
INSERT INTO creative_ads (project_id, concept_id, code, file_name, pos, fmt, lang, has_cta, render_code, draft, kind, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateAdShape :exec
UPDATE creative_ads
SET concept_id = ?, file_name = ?, fmt = ?, lang = ?, has_cta = ?, render_code = ?, updated_at = ?
WHERE id = ?;

-- name: SetAdKind :exec
UPDATE creative_ads SET kind = ?, file_name = ?, updated_at = ? WHERE id = ?;

-- name: SaveAdDraft :exec
UPDATE creative_ads SET draft = ?, render_code = ?, updated_at = ? WHERE id = ?;

-- name: ListSiblingAds :many
SELECT * FROM creative_ads
WHERE concept_id = ? AND lang = ? AND has_cta = ? AND id != ?
ORDER BY pos;

-- name: LatestVersion :one
SELECT * FROM creative_ad_versions WHERE ad_id = ? ORDER BY version DESC LIMIT 1;

-- name: ListLatestVersions :many
SELECT v.* FROM creative_ad_versions v
JOIN creative_ads a ON a.id = v.ad_id
WHERE a.project_id = ?
  AND v.version = (SELECT MAX(x.version) FROM creative_ad_versions x WHERE x.ad_id = v.ad_id);

-- name: InsertVersion :one
INSERT INTO creative_ad_versions (ad_id, version, file_id, preview_file_id, design, source, method, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListCurrentDecisions :many
SELECT d.*, v.ad_id FROM creative_decisions d
JOIN creative_ad_versions v ON v.id = d.version_id
JOIN creative_ads a ON a.id = v.ad_id
WHERE a.project_id = ?
  AND v.version = (SELECT MAX(x.version) FROM creative_ad_versions x WHERE x.ad_id = v.ad_id)
ORDER BY d.created_at, d.id;

-- name: InsertDecision :exec
INSERT INTO creative_decisions (version_id, name, status, user_id, created_at) VALUES (?, ?, ?, ?, ?);

-- name: CountDecisions :one
SELECT CAST(COUNT(*) AS INTEGER) AS total,
       CAST(COALESCE(SUM(d.created_at > sqlc.arg(since)), 0) AS INTEGER) AS recent
FROM creative_decisions d
JOIN creative_ad_versions v ON v.id = d.version_id
JOIN creative_ads a ON a.id = v.ad_id
WHERE a.project_id = sqlc.arg(project_id);

-- name: ListComments :many
SELECT c.*, v.version FROM creative_comments c
JOIN creative_ads a ON a.id = c.ad_id
JOIN creative_ad_versions v ON v.id = c.version_id
WHERE a.project_id = ?
ORDER BY c.created_at, c.id;

-- name: InsertComment :one
INSERT INTO creative_comments (ad_id, version_id, name, body, pin_x, pin_y, user_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: CountComments :one
SELECT CAST(COUNT(*) AS INTEGER) AS total,
       CAST(COALESCE(SUM(c.created_at > sqlc.arg(since)), 0) AS INTEGER) AS recent
FROM creative_comments c
JOIN creative_ads a ON a.id = c.ad_id
WHERE a.project_id = sqlc.arg(project_id);

-- name: GetComment :one
SELECT * FROM creative_comments WHERE id = ?;

-- name: SetCommentResolved :exec
UPDATE creative_comments SET resolved_at = ?, resolved_by = ? WHERE id = ?;

-- name: ListLatestPicks :many
SELECT p.*, k.code AS concept_code FROM creative_render_picks p
JOIN creative_concepts k ON k.id = p.concept_id
WHERE k.project_id = ?
  AND p.id = (SELECT MAX(x.id) FROM creative_render_picks x WHERE x.concept_id = p.concept_id);

-- name: InsertPick :exec
INSERT INTO creative_render_picks (concept_id, render_code, name, created_at) VALUES (?, ?, ?, ?);

-- name: CountPicks :one
SELECT CAST(COUNT(*) AS INTEGER) AS total,
       CAST(COALESCE(SUM(p.created_at > sqlc.arg(since)), 0) AS INTEGER) AS recent
FROM creative_render_picks p
JOIN creative_concepts k ON k.id = p.concept_id
WHERE k.project_id = sqlc.arg(project_id);
