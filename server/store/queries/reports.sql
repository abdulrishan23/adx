-- name: CreateReportSchedule :one
INSERT INTO report_schedules (workspace_id, name, cadence, enabled, params, recipients, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateReportSchedule :one
UPDATE report_schedules SET name = ?, cadence = ?, enabled = ?, params = ?, recipients = ?, updated_at = ?
WHERE id = ? AND workspace_id = ?
RETURNING *;

-- name: GetReportSchedule :one
SELECT * FROM report_schedules WHERE id = ? AND workspace_id = ?;

-- name: ListReportSchedules :many
SELECT * FROM report_schedules WHERE workspace_id = ? ORDER BY cadence, name;

-- name: ListEnabledSchedulesAllWorkspaces :many
SELECT * FROM report_schedules WHERE enabled = 1 ORDER BY workspace_id, id;

-- name: GetReportByOccurrence :one
SELECT * FROM reports WHERE workspace_id = ? AND occurrence_key = ?;

-- name: CreateReport :one
INSERT INTO reports
    (workspace_id, schedule_id, occurrence_key, cadence, period_start, period_end, params, formula_version, result,
     status, error_safe, delivery_status, delivery_detail, generated_at, requested_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetReport :one
SELECT * FROM reports WHERE id = ? AND workspace_id = ?;

-- name: ListReports :many
SELECT * FROM reports WHERE workspace_id = ? ORDER BY generated_at DESC, id DESC LIMIT ?;

-- name: SetReportDelivery :execrows
UPDATE reports SET delivery_status = ?, delivery_detail = ?, delivered_at = ?
WHERE id = ? AND delivery_status = 'pending';

-- name: GetCurrentAlertConfig :one
SELECT * FROM alert_configs WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreateAlertConfig :one
INSERT INTO alert_configs (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetOpenAlert :one
SELECT * FROM alerts WHERE workspace_id = ? AND check_name = ? AND subject_key = ? AND state IN ('open', 'acknowledged');

-- name: CreateAlert :one
INSERT INTO alerts
    (workspace_id, check_name, subject_key, subject_label, severity, state, message, evidence, config_version, occurrences, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, 'open', ?, ?, ?, 1, ?, ?)
RETURNING *;

-- name: TouchAlert :one
UPDATE alerts SET occurrences = occurrences + 1, last_seen_at = ?, message = ?, evidence = ?, severity = ?
WHERE id = ?
RETURNING *;

-- name: ListAlerts :many
SELECT * FROM alerts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(state) IS NULL OR state = sqlc.narg(state))
ORDER BY CASE severity WHEN 'urgent' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, last_seen_at DESC
LIMIT sqlc.arg(max);

-- name: AcknowledgeAlert :one
UPDATE alerts SET state = 'acknowledged', acknowledged_by = ?, acknowledged_at = ?
WHERE id = ? AND workspace_id = ? AND state = 'open'
RETURNING *;

-- name: ResolveAlert :one
UPDATE alerts SET state = 'resolved', resolved_at = ?
WHERE id = ? AND workspace_id = ? AND state IN ('open', 'acknowledged')
RETURNING *;
