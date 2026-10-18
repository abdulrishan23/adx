-- name: UpsertAdCampaign :one
INSERT INTO ad_campaigns
    (workspace_id, account_id, platform, external_id, name, status, channel_type, budget_minor, budget_period, currency, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, external_id) DO UPDATE SET
    name = excluded.name, status = excluded.status, channel_type = excluded.channel_type,
    budget_minor = excluded.budget_minor, budget_period = excluded.budget_period, currency = excluded.currency,
    last_seen_at = excluded.last_seen_at, removed_at = NULL
RETURNING *;

-- name: GetAdCampaignByExternalID :one
SELECT * FROM ad_campaigns WHERE account_id = ? AND external_id = ?;

-- name: ListAdCampaigns :many
SELECT * FROM ad_campaigns
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id))
  AND (sqlc.narg(platform) IS NULL OR platform = sqlc.narg(platform))
  AND removed_at IS NULL
ORDER BY platform, name;

-- name: SetAdCampaignProject :exec
UPDATE ad_campaigns SET project_id = ? WHERE id = ?;

-- name: UpsertCampaignDailyMetrics :exec
INSERT INTO campaign_daily_metrics
    (workspace_id, account_id, campaign_id, platform, day, currency, cost_minor, impressions, clicks,
     conversions, conversions_value_minor, all_conversions, platform_leads, reach, observed_at, run_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (campaign_id, day) DO UPDATE SET
    currency = excluded.currency, cost_minor = excluded.cost_minor, impressions = excluded.impressions, clicks = excluded.clicks,
    conversions = excluded.conversions, conversions_value_minor = excluded.conversions_value_minor,
    all_conversions = excluded.all_conversions, platform_leads = excluded.platform_leads, reach = excluded.reach,
    observed_at = excluded.observed_at, run_id = excluded.run_id;

-- name: ListCampaignDailyMetrics :many
SELECT m.*, c.name AS campaign_name, c.external_id AS campaign_external_id, c.project_id
FROM campaign_daily_metrics m
JOIN ad_campaigns c ON c.id = m.campaign_id
WHERE m.workspace_id = sqlc.arg(workspace_id)
  AND m.day >= sqlc.arg(from_day) AND m.day <= sqlc.arg(to_day)
  AND (sqlc.narg(account_id) IS NULL OR m.account_id = sqlc.narg(account_id))
  AND (sqlc.narg(platform) IS NULL OR m.platform = sqlc.narg(platform))
ORDER BY m.day, m.platform, c.name;

-- name: CountMetricRows :one
SELECT count(*) FROM campaign_daily_metrics WHERE account_id = ?;

-- name: UpsertConversionAction :exec
INSERT INTO conversion_actions
    (workspace_id, account_id, external_id, name, type, category, status, counting_type, include_in_conversions, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, external_id) DO UPDATE SET
    name = excluded.name, type = excluded.type, category = excluded.category, status = excluded.status,
    counting_type = excluded.counting_type, include_in_conversions = excluded.include_in_conversions, last_seen_at = excluded.last_seen_at;

-- name: ListConversionActions :many
SELECT * FROM conversion_actions WHERE account_id = ? ORDER BY name;

-- name: UpsertAccountFreshness :exec
INSERT INTO account_freshness (account_id, workspace_id, last_success_at, observed_through, last_run_id, last_partial, last_error)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id) DO UPDATE SET
    last_success_at = COALESCE(excluded.last_success_at, account_freshness.last_success_at),
    observed_through = COALESCE(excluded.observed_through, account_freshness.observed_through),
    last_run_id = excluded.last_run_id, last_partial = excluded.last_partial, last_error = excluded.last_error;

-- name: ListAccountFreshness :many
SELECT * FROM account_freshness WHERE workspace_id = ?;

-- name: GetAccountFreshness :one
SELECT * FROM account_freshness WHERE account_id = ?;

-- name: ListLeadsAddedBetween :many
SELECT * FROM leads
WHERE workspace_id = sqlc.arg(workspace_id) AND removed_at IS NULL
  AND provider_added_at >= sqlc.arg(from_at) AND provider_added_at < sqlc.arg(to_at)
  AND (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id))
ORDER BY provider_added_at, id;

-- name: ListLeadAttributionsBetween :many
SELECT la.* FROM lead_attributions la
JOIN leads l ON l.id = la.lead_id
WHERE l.workspace_id = sqlc.arg(workspace_id) AND l.removed_at IS NULL
  AND l.provider_added_at >= sqlc.arg(from_at) AND l.provider_added_at < sqlc.arg(to_at)
ORDER BY la.lead_id, la.is_first DESC, la.id;

-- name: ListAdAccounts :many
SELECT sqlc.embed(accounts), sqlc.embed(credentials)
FROM accounts JOIN credentials ON credentials.id = accounts.credential_id
WHERE accounts.type IN ('google_ads', 'meta')
ORDER BY accounts.type, accounts.external_id;

-- name: ListAllAdCampaigns :many
SELECT * FROM ad_campaigns WHERE workspace_id = ? AND removed_at IS NULL ORDER BY id;

-- name: ListAdCampaignsForProject :many
SELECT * FROM ad_campaigns
WHERE workspace_id = ? AND project_id = ? AND removed_at IS NULL
ORDER BY platform, name, id;

-- name: SumCampaignSpendForProject :many
SELECT m.campaign_id, CAST(SUM(m.cost_minor) AS INTEGER) AS spent_minor, CAST(COUNT(*) AS INTEGER) AS days
FROM campaign_daily_metrics m
JOIN ad_campaigns c ON c.id = m.campaign_id
WHERE c.workspace_id = sqlc.arg(workspace_id) AND c.project_id = sqlc.arg(project_id)
  AND m.day >= sqlc.arg(from_day) AND m.day <= sqlc.arg(to_day)
GROUP BY m.campaign_id;
