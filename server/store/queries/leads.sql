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

-- A lead row with what the screens read beside it: the person it is linked
-- to (the active link, never a reversed one), that person's latest
-- temperature, and the best match level among its attribution records
-- (1 click id ... 5 none; null when it has no record). person_id is 0 when
-- the lead is not linked to a person yet.
-- name: ListLeads :many
SELECT sqlc.embed(leads),
    coalesce((SELECT k.person_id FROM person_links k
       WHERE k.workspace_id = leads.workspace_id AND k.kind = 'lead' AND k.ref = CAST(leads.id AS TEXT) AND k.reversed_at IS NULL
       LIMIT 1), 0) AS person_id,
    (SELECT sa.temperature FROM score_assessments sa
       JOIN person_links k ON k.person_id = sa.person_id
       WHERE k.workspace_id = leads.workspace_id AND k.kind = 'lead' AND k.ref = CAST(leads.id AS TEXT) AND k.reversed_at IS NULL
       ORDER BY sa.assessed_at DESC, sa.id DESC LIMIT 1) AS temperature,
    (SELECT min(CASE la.match_level WHEN 'click_id' THEN 1 WHEN 'platform_lead_id' THEN 2 WHEN 'hashed_contact' THEN 3 WHEN 'utm' THEN 4 ELSE 5 END)
       FROM lead_attributions la WHERE la.lead_id = leads.id) AS match_rank
FROM leads
WHERE leads.workspace_id = sqlc.arg(workspace_id)
  AND leads.removed_at IS NULL
  AND (sqlc.narg(account_id) IS NULL OR leads.account_id = sqlc.narg(account_id))
  AND (sqlc.narg(stage) IS NULL OR leads.stage = sqlc.narg(stage))
  AND (sqlc.narg(status_raw) IS NULL OR leads.status_raw = sqlc.narg(status_raw))
  AND (sqlc.narg(after_id) IS NULL OR leads.id > sqlc.narg(after_id))
ORDER BY leads.id
LIMIT sqlc.arg(max);

-- name: GetLeadDetail :one
SELECT sqlc.embed(leads),
    coalesce((SELECT k.person_id FROM person_links k
       WHERE k.workspace_id = leads.workspace_id AND k.kind = 'lead' AND k.ref = CAST(leads.id AS TEXT) AND k.reversed_at IS NULL
       LIMIT 1), 0) AS person_id,
    (SELECT sa.temperature FROM score_assessments sa
       JOIN person_links k ON k.person_id = sa.person_id
       WHERE k.workspace_id = leads.workspace_id AND k.kind = 'lead' AND k.ref = CAST(leads.id AS TEXT) AND k.reversed_at IS NULL
       ORDER BY sa.assessed_at DESC, sa.id DESC LIMIT 1) AS temperature,
    (SELECT min(CASE la.match_level WHEN 'click_id' THEN 1 WHEN 'platform_lead_id' THEN 2 WHEN 'hashed_contact' THEN 3 WHEN 'utm' THEN 4 ELSE 5 END)
       FROM lead_attributions la WHERE la.lead_id = leads.id) AS match_rank
FROM leads
WHERE leads.id = ? AND leads.workspace_id = ? AND leads.removed_at IS NULL;

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
