-- name: UpsertLaunchPick :one
INSERT INTO launch_picks (workspace_id, plan_id, plan_version, line_index, channel, campaign_id, picked_by, picked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (plan_id, plan_version, line_index) DO UPDATE SET
    campaign_id = excluded.campaign_id, picked_by = excluded.picked_by, picked_at = excluded.picked_at
RETURNING *;

-- name: ListLaunchPicks :many
SELECT * FROM launch_picks WHERE workspace_id = ? AND plan_id = ? AND plan_version = ? ORDER BY line_index;

-- name: ListAcceptedPlanVersionsForProject :many
SELECT sqlc.embed(plan_versions), sqlc.embed(plans)
FROM plan_versions
JOIN plans ON plans.id = plan_versions.plan_id
WHERE plans.workspace_id = ? AND plans.project_id = ? AND plan_versions.state = 'accepted'
ORDER BY plans.id, plan_versions.version DESC;
