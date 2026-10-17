-- name: GetCurrentSupervisorConfig :one
SELECT * FROM supervisor_configs WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreateSupervisorConfig :one
INSERT INTO supervisor_configs (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateRecommendation :one
INSERT INTO recommendations (workspace_id, proposal_id, source, capability, objective, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (proposal_id) DO UPDATE SET source = excluded.source, capability = excluded.capability, objective = excluded.objective
RETURNING *;

-- name: GetRecommendationByProposal :one
SELECT * FROM recommendations WHERE proposal_id = ?;

-- name: CountRecommendationsSince :one
SELECT COUNT(*) FROM recommendations WHERE workspace_id = ? AND capability = ? AND created_at >= ?;

-- name: ListRecommendations :many
SELECT * FROM recommendations WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: CreateSupervisorConflict :one
INSERT INTO supervisor_conflicts (workspace_id, envelope_id, proposal_ids, state, detail, resolution, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSupervisorConflict :one
SELECT * FROM supervisor_conflicts WHERE id = ? AND workspace_id = ?;

-- name: ListSupervisorConflicts :many
SELECT * FROM supervisor_conflicts WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: ListOpenSupervisorConflictsForEnvelope :many
SELECT * FROM supervisor_conflicts WHERE envelope_id = ? AND state = 'escalated';

-- name: ResolveSupervisorConflict :execrows
UPDATE supervisor_conflicts SET state = 'resolved', resolution = ?, resolved_by = ?, resolved_at = ? WHERE id = ? AND state = 'escalated';

-- name: ListLiveProposalsForEnvelope :many
SELECT * FROM proposals WHERE envelope_id = ? AND state IN ('proposed', 'approved') ORDER BY id;

-- name: ListFieldOutboxConflicts :many
SELECT * FROM crm_field_outbox WHERE workspace_id = ? AND state = 'conflict' ORDER BY id DESC LIMIT ?;

-- name: ListPendingCallSessions :many
SELECT * FROM call_sessions WHERE workspace_id = ? AND state = 'ended' AND review_status = 'pending' ORDER BY id DESC LIMIT ?;

-- name: ListWorkflowTasksNeedingPeople :many
SELECT t.* FROM workflow_tasks t JOIN workflow_runs r ON r.id = t.run_id
WHERE r.workspace_id = ? AND t.state IN ('blocked', 'failed') ORDER BY t.id DESC LIMIT ?;

-- name: ListFailedRuns :many
SELECT * FROM runs WHERE workspace_id = ? AND state IN ('failed', 'interrupted') ORDER BY id DESC LIMIT ?;
