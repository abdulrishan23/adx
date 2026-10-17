-- name: GetLeadByExternalID :one
SELECT * FROM leads WHERE account_id = ? AND external_id = ?;

-- name: UpsertLead :one
INSERT INTO leads
    (workspace_id, account_id, external_id, first_name, last_name, email, phone, phone_e164, dial_code,
     source_raw, project_raw, project_id, status_raw, status_observed_at, status_missing_since, stage, mapping_version,
     provider_added_at, provider_updated_at, first_seen_at, last_seen_at, last_seen_run_id, evidence_id, removed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
ON CONFLICT (account_id, external_id) DO UPDATE SET
    first_name = excluded.first_name,
    last_name = excluded.last_name,
    email = excluded.email,
    phone = excluded.phone,
    phone_e164 = excluded.phone_e164,
    dial_code = excluded.dial_code,
    source_raw = excluded.source_raw,
    project_raw = excluded.project_raw,
    project_id = excluded.project_id,
    status_raw = excluded.status_raw,
    status_observed_at = excluded.status_observed_at,
    status_missing_since = excluded.status_missing_since,
    stage = excluded.stage,
    mapping_version = excluded.mapping_version,
    provider_added_at = excluded.provider_added_at,
    provider_updated_at = excluded.provider_updated_at,
    last_seen_at = excluded.last_seen_at,
    last_seen_run_id = excluded.last_seen_run_id,
    evidence_id = excluded.evidence_id,
    removed_at = NULL
RETURNING *;

-- name: MarkLeadsUnseenRemoved :execrows
UPDATE leads SET removed_at = sqlc.arg(removed_at)
WHERE account_id = sqlc.arg(account_id) AND removed_at IS NULL
  AND (last_seen_run_id IS NULL OR last_seen_run_id != sqlc.arg(run_id));

-- name: ListLeads :many
SELECT * FROM leads
WHERE workspace_id = sqlc.arg(workspace_id)
  AND removed_at IS NULL
  AND (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id))
  AND (sqlc.narg(stage) IS NULL OR stage = sqlc.narg(stage))
  AND (sqlc.narg(status_raw) IS NULL OR status_raw = sqlc.narg(status_raw))
  AND (sqlc.narg(after_id) IS NULL OR id > sqlc.narg(after_id))
ORDER BY id
LIMIT sqlc.arg(max);

-- name: ListLeadsByAccount :many
SELECT * FROM leads WHERE account_id = ? ORDER BY id;

-- name: CountLeadsByStatus :many
SELECT status_raw, stage, count(*) AS n
FROM leads
WHERE workspace_id = ? AND removed_at IS NULL AND (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id))
GROUP BY status_raw, stage
ORDER BY n DESC;

-- name: LeadCoverage :one
SELECT
    count(*) AS total,
    sum(CASE WHEN status_raw IS NOT NULL THEN 1 ELSE 0 END) AS with_status,
    sum(CASE WHEN stage IS NOT NULL THEN 1 ELSE 0 END) AS with_stage,
    sum(CASE WHEN project_id IS NOT NULL THEN 1 ELSE 0 END) AS with_project,
    sum(CASE WHEN phone_e164 IS NOT NULL THEN 1 ELSE 0 END) AS with_phone,
    sum(CASE WHEN status_missing_since IS NOT NULL THEN 1 ELSE 0 END) AS status_missing
FROM leads
WHERE workspace_id = ? AND removed_at IS NULL AND (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id));

-- name: CountSharedPhones :one
SELECT count(*) FROM (
    SELECT phone_e164 FROM leads
    WHERE workspace_id = ? AND removed_at IS NULL AND phone_e164 IS NOT NULL
    GROUP BY phone_e164 HAVING count(*) > 1
);

-- name: RestageLeads :execrows
UPDATE leads SET stage = ?, mapping_version = ?
WHERE workspace_id = ? AND status_raw IS ? AND removed_at IS NULL;

-- name: InsertLeadAttribution :exec
INSERT INTO lead_attributions
    (lead_id, observed_at, is_first, utm_source, utm_medium, utm_campaign, utm_content, utm_term, gclid, fbclid,
     referrer, landing_url, match_level, match_confidence, matched_platform, matched_campaign_ref, payload_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (lead_id, payload_hash) DO NOTHING;

-- name: ListLeadAttributions :many
SELECT * FROM lead_attributions WHERE lead_id = ? ORDER BY observed_at, id;

-- name: CountAttributionLevels :many
SELECT la.match_level, count(DISTINCT la.lead_id) AS n
FROM lead_attributions la JOIN leads l ON l.id = la.lead_id
WHERE l.workspace_id = ? AND l.removed_at IS NULL
GROUP BY la.match_level;

-- name: GetCurrentFunnelMapping :one
SELECT * FROM funnel_mappings WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreateFunnelMapping :one
INSERT INTO funnel_mappings (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;
