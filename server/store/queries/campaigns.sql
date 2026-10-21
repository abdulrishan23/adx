-- name: InsertCampaign :one
INSERT INTO campaigns (workspace_id, project_id, name, goal, audience, avoid, offer_id, starts_on, ends_on, budget_minor, currency, channels, markets, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCampaign :one
UPDATE campaigns
SET name = ?, goal = ?, audience = ?, avoid = ?, offer_id = ?, starts_on = ?, ends_on = ?, budget_minor = ?, currency = ?, channels = ?, markets = ?, updated_at = ?
WHERE id = ? AND workspace_id = ?
RETURNING *;

-- name: GetCampaign :one
SELECT * FROM campaigns WHERE id = ? AND workspace_id = ?;

-- name: ListCampaigns :many
SELECT * FROM campaigns WHERE workspace_id = ?
ORDER BY starts_on DESC, created_at DESC;

-- name: ListCampaignsForProject :many
SELECT * FROM campaigns WHERE workspace_id = ? AND project_id = ?
ORDER BY starts_on DESC, created_at DESC;

-- name: ListConceptsForCampaign :many
SELECT * FROM creative_concepts WHERE campaign_id = ? ORDER BY pos, code;

-- name: SetConceptCampaign :exec
UPDATE creative_concepts SET campaign_id = ? WHERE id = ?;

-- name: ListPagesForCampaign :many
SELECT * FROM pages WHERE campaign_id = ? ORDER BY language, id;

-- name: ListPlansForCampaign :many
SELECT * FROM plans WHERE campaign_id = ? ORDER BY id;
