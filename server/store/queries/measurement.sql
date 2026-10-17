-- name: CreateMeasurementEvent :one
INSERT INTO measurement_events (workspace_id, kind, source, source_ref, page_id, project_id, plan_id, test, dedupe_key, duplicate_of_id, counts, attribution, occurred_at, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMeasurementEventBySourceRef :one
SELECT * FROM measurement_events WHERE source_ref = ?;

-- name: GetMeasurementEvent :one
SELECT * FROM measurement_events WHERE id = ? AND workspace_id = ?;

-- name: FindMeasurementEventByDedupe :one
SELECT * FROM measurement_events
WHERE workspace_id = ? AND dedupe_key = ? AND occurred_at >= ? AND duplicate_of_id IS NULL
ORDER BY occurred_at, id LIMIT 1;

-- name: ListMeasurementEvents :many
SELECT * FROM measurement_events
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(page_id) IS NULL OR page_id = sqlc.narg(page_id))
ORDER BY occurred_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: CountCountingEvents :one
SELECT count(*) FROM measurement_events WHERE workspace_id = ? AND test = 0 AND counts = 1;

-- name: ListUnrecordedSubmissions :many
SELECT s.* FROM form_submissions s
WHERE s.workspace_id = ? AND s.state = 'received'
  AND NOT EXISTS (SELECT 1 FROM measurement_events e WHERE e.source_ref = 'form_submission:' || s.id)
ORDER BY s.id;

-- name: CreateMeasurementCheck :one
INSERT INTO measurement_checks (workspace_id, plan_id, plan_version, kind, page_id, account_id, state, detail, submission_id, event_id, evidence_id, checked_by, checked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListMeasurementChecks :many
SELECT * FROM measurement_checks WHERE workspace_id = ? AND plan_id = ? AND plan_version = ? ORDER BY checked_at DESC, id DESC;

-- name: ListPagesByPlan :many
SELECT * FROM pages WHERE workspace_id = ? AND plan_id = ? ORDER BY id;
