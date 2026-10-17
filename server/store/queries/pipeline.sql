-- name: GetCurrentPipelineConfig :one
SELECT * FROM pipeline_configs WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreatePipelineConfig :one
INSERT INTO pipeline_configs (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateOpportunity :one
INSERT INTO opportunities (workspace_id, enquiry_id, person_id, project_id, stage, stage_set_by, state, owner_user_id, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?, ?, ?)
RETURNING *;

-- name: GetOpportunity :one
SELECT * FROM opportunities WHERE id = ? AND workspace_id = ?;

-- name: GetOpportunityByEnquiry :one
SELECT * FROM opportunities WHERE enquiry_id = ?;

-- name: ListOpportunities :many
SELECT * FROM opportunities
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(stage) IS NULL OR stage = sqlc.narg(stage))
  AND (sqlc.narg(state) IS NULL OR state = sqlc.narg(state))
  AND (sqlc.narg(owner_user_id) IS NULL OR owner_user_id = sqlc.narg(owner_user_id))
ORDER BY updated_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: ListOpportunitiesForPerson :many
SELECT * FROM opportunities WHERE person_id = ? ORDER BY id;

-- name: CountOpportunities :one
SELECT COUNT(*) FROM opportunities WHERE workspace_id = ?;

-- name: UpdateOpportunityStage :execrows
UPDATE opportunities SET stage = ?, stage_set_by = ?, state = ?, version = version + 1, updated_at = ?, closed_at = ?
WHERE id = ? AND version = ?;

-- name: UpdateOpportunityValue :execrows
UPDATE opportunities SET value_minor = ?, currency = ?, version = version + 1, updated_at = ? WHERE id = ? AND version = ?;

-- name: SetOpportunityOwner :exec
UPDATE opportunities SET owner_user_id = ?, version = version + 1, updated_at = ? WHERE id = ?;

-- name: CreateAssignment :one
INSERT INTO opportunity_assignments (opportunity_id, user_id, assigned_by, reason, assigned_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: EndActiveAssignments :exec
UPDATE opportunity_assignments SET ended_at = ? WHERE opportunity_id = ? AND ended_at IS NULL;

-- name: ListAssignments :many
SELECT * FROM opportunity_assignments WHERE opportunity_id = ? ORDER BY id;

-- name: CreateInteraction :one
INSERT INTO interactions (workspace_id, person_id, opportunity_id, kind, summary, occurred_at, recorded_by, evidence_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListInteractionsForPerson :many
SELECT * FROM interactions WHERE person_id = ? ORDER BY occurred_at DESC, id DESC LIMIT ?;

-- name: CreateFollowUpTask :one
INSERT INTO follow_up_tasks (workspace_id, opportunity_id, person_id, title, due_at, state, assignee_user_id, occurrence_key, created_by, created_at)
VALUES (?, ?, ?, ?, ?, 'open', ?, ?, ?, ?)
RETURNING *;

-- name: GetFollowUpTask :one
SELECT * FROM follow_up_tasks WHERE id = ? AND workspace_id = ?;

-- name: GetFollowUpTaskByKey :one
SELECT * FROM follow_up_tasks WHERE workspace_id = ? AND occurrence_key = ?;

-- name: ListFollowUpTasks :many
SELECT * FROM follow_up_tasks
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(state) IS NULL OR state = sqlc.narg(state))
  AND (sqlc.narg(assignee_user_id) IS NULL OR assignee_user_id = sqlc.narg(assignee_user_id))
ORDER BY CASE WHEN due_at IS NULL THEN 1 ELSE 0 END, due_at, id LIMIT sqlc.arg(max);

-- name: ListFollowUpTasksForOpportunity :many
SELECT * FROM follow_up_tasks WHERE opportunity_id = ? ORDER BY id;

-- name: SetFollowUpTaskState :execrows
UPDATE follow_up_tasks SET state = ?, done_at = ?, done_by = ? WHERE id = ? AND state = 'open';

-- name: CreateImportBatch :one
INSERT INTO crm_import_batches (workspace_id, source, state, config_version, total, created_n, updated_n, skipped_n, conflict_n, created_by, created_at, updated_at)
VALUES (?, ?, 'previewed', ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetImportBatch :one
SELECT * FROM crm_import_batches WHERE id = ? AND workspace_id = ?;

-- name: ListImportBatches :many
SELECT * FROM crm_import_batches WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: SetImportBatchState :exec
UPDATE crm_import_batches SET state = ?, cursor = ?, error = ?, updated_at = ?, applied_at = COALESCE(applied_at, ?) WHERE id = ?;

-- name: CreateImportRow :exec
INSERT INTO crm_import_rows (batch_id, position, enquiry_id, lead_id, action, detail)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListImportRows :many
SELECT * FROM crm_import_rows WHERE batch_id = ? ORDER BY position;

-- name: ListImportRowsFrom :many
SELECT * FROM crm_import_rows WHERE batch_id = ? AND position >= ? ORDER BY position LIMIT ?;

-- name: MarkImportRowApplied :exec
UPDATE crm_import_rows SET applied_at = ? WHERE id = ?;

-- name: ListEnquiriesWithLeads :many
SELECT e.*, l.id AS lead_id, l.stage AS lead_stage, l.status_raw AS lead_status, l.external_id AS lead_external_id, l.account_id AS lead_account_id
FROM enquiries e
LEFT JOIN person_links k ON k.kind = 'lead' AND k.person_id = e.person_id AND k.reversed_at IS NULL AND e.source_kind = 'lead' AND k.ref = e.source_ref
LEFT JOIN leads l ON l.id = CAST(k.ref AS INTEGER)
WHERE e.workspace_id = ? ORDER BY e.id;

-- name: UpsertProviderMapping :one
INSERT INTO provider_mappings (workspace_id, provider, kind, local_id, external_id, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (workspace_id, provider, kind, local_id) DO UPDATE SET external_id = excluded.external_id
RETURNING *;

-- name: ListProviderMappings :many
SELECT * FROM provider_mappings WHERE workspace_id = ? AND provider = ? AND kind = ? ORDER BY local_id;

-- name: GetProviderMappingByExternal :one
SELECT * FROM provider_mappings WHERE workspace_id = ? AND provider = ? AND kind = ? AND external_id = ?;
