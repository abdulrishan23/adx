-- name: CreatePage :one
INSERT INTO pages (workspace_id, brand_id, project_id, plan_id, plan_version, slug, name, language, test_token, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPage :one
SELECT * FROM pages WHERE id = ? AND workspace_id = ?;

-- name: GetPageBySlug :one
SELECT * FROM pages WHERE slug = ?;

-- name: ListPages :many
SELECT * FROM pages WHERE workspace_id = ? ORDER BY updated_at DESC, id DESC;

-- name: TouchPage :exec
UPDATE pages SET updated_at = ? WHERE id = ?;

-- name: CreatePageVersion :one
INSERT INTO page_versions (page_id, version, document, note, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPageVersion :one
SELECT * FROM page_versions WHERE page_id = ? AND version = ?;

-- name: GetPageVersionByID :one
SELECT * FROM page_versions WHERE id = ?;

-- name: GetLatestPageVersion :one
SELECT * FROM page_versions WHERE page_id = ? ORDER BY version DESC LIMIT 1;

-- name: ListPageVersions :many
SELECT * FROM page_versions WHERE page_id = ? ORDER BY version;

-- name: CreatePageDeployment :one
INSERT INTO page_deployments (page_id, version_id, adapter, state, url, output_sha256, rolled_back_from_id, error_safe, deployed_by, deployed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: SupersedePageDeployments :exec
UPDATE page_deployments SET state = 'superseded' WHERE page_id = ? AND state = 'published' AND id != ?;

-- name: GetCurrentPageDeployment :one
SELECT * FROM page_deployments WHERE page_id = ? AND state = 'published' ORDER BY deployed_at DESC, id DESC LIMIT 1;

-- name: ListPageDeployments :many
SELECT * FROM page_deployments WHERE page_id = ? ORDER BY deployed_at DESC, id DESC;

-- name: CreateFormSubmission :one
INSERT INTO form_submissions (workspace_id, page_id, version_id, project_id, state, reason, test, fields, attribution, ip_hash, user_agent, evidence_id, received_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListFormSubmissions :many
SELECT * FROM form_submissions WHERE page_id = ? ORDER BY received_at DESC, id DESC LIMIT ?;

-- name: CountFormSubmissionsSince :one
SELECT count(*) FROM form_submissions WHERE page_id = ? AND received_at >= ?;

-- name: LatestTestSubmissionForVersion :one
SELECT * FROM form_submissions WHERE version_id = ? AND test = 1 ORDER BY received_at DESC, id DESC LIMIT 1;
