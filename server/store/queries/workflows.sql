-- name: CreateWorkflowRun :one
INSERT INTO workflow_runs (workspace_id, definition_key, definition_version, state, params, budget_tokens, created_by, created_at, updated_at)
VALUES (?, ?, ?, 'running', ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkflowRun :one
SELECT * FROM workflow_runs WHERE id = ? AND workspace_id = ?;

-- name: ListWorkflowRuns :many
SELECT * FROM workflow_runs WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: ListWorkflowRunsByState :many
SELECT * FROM workflow_runs WHERE state = ? ORDER BY id LIMIT ?;

-- name: SetWorkflowRunState :execrows
UPDATE workflow_runs SET state = ?, reason = ?, updated_at = ?, finished_at = COALESCE(finished_at, ?) WHERE id = ? AND state = ?;

-- name: AddWorkflowRunSpend :exec
UPDATE workflow_runs SET spent_tokens = spent_tokens + ?, updated_at = ? WHERE id = ?;

-- name: CreateWorkflowTask :one
INSERT INTO workflow_tasks (run_id, key, iteration, skill, agent, depends_on, state, max_attempts, input, external, operation_key, loop_max, budget_tokens, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkflowTask :one
SELECT * FROM workflow_tasks WHERE id = ?;

-- name: ListWorkflowTasks :many
SELECT * FROM workflow_tasks WHERE run_id = ? ORDER BY id;

-- name: ListWorkflowTasksByState :many
SELECT * FROM workflow_tasks WHERE run_id = ? AND state = ? ORDER BY id;

-- name: LeaseWorkflowTask :execrows
UPDATE workflow_tasks SET state = 'leased', lease_owner = ?, lease_until = ?, attempt = attempt + 1, started_at = COALESCE(started_at, ?), updated_at = ?
WHERE id = ? AND state = 'ready';

-- name: SetWorkflowTaskState :execrows
UPDATE workflow_tasks SET state = ?, reason = ?, lease_owner = NULL, lease_until = NULL, updated_at = ?, finished_at = ? WHERE id = ? AND state = ?;

-- name: FinishWorkflowTask :execrows
UPDATE workflow_tasks SET state = 'done', output = ?, output_version = output_version + 1, input_hash = ?, spent_tokens = spent_tokens + ?, reason = '', lease_owner = NULL, lease_until = NULL, updated_at = ?, finished_at = ?
WHERE id = ? AND state = ? AND lease_owner IS ?;

-- name: SetWorkflowTaskResolvedInput :exec
UPDATE workflow_tasks SET resolved_input = ?, updated_at = ? WHERE id = ?;

-- name: SetWorkflowTaskExternalRef :execrows
UPDATE workflow_tasks SET external_ref = ?, updated_at = ? WHERE id = ? AND external_ref = '';

-- name: ReleaseExpiredWorkflowLeases :many
UPDATE workflow_tasks SET state = 'ready', lease_owner = NULL, lease_until = NULL, reason = 'lease expired; resumed', updated_at = ?
WHERE state = 'leased' AND lease_until < ? RETURNING *;

-- name: InvalidateWorkflowTask :execrows
UPDATE workflow_tasks SET state = ?, reason = ?, updated_at = ? WHERE id = ? AND state IN ('done', 'ready', 'pending', 'blocked');

-- name: CreateWorkflowEvent :exec
INSERT INTO workflow_events (run_id, task_id, kind, detail, at) VALUES (?, ?, ?, ?, ?);

-- name: ListWorkflowEvents :many
SELECT * FROM workflow_events WHERE run_id = ? ORDER BY id;

-- name: SumWorkflowSpendSince :one
SELECT CAST(COALESCE(sum(spent_tokens), 0) AS INTEGER) AS spent_tokens,
       CAST(COALESCE(sum(CASE WHEN state = 'blocked' AND reason = 'budget spent' THEN 1 ELSE 0 END), 0) AS INTEGER) AS blocked_on_budget
FROM workflow_runs WHERE workspace_id = ? AND created_at >= ?;
