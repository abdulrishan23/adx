-- name: UpsertQualificationLabel :one
INSERT INTO qualification_labels (workspace_id, call_id, outcome, answers, note, labeled_by, labeled_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (call_id) DO UPDATE SET outcome = excluded.outcome, answers = excluded.answers, note = excluded.note, labeled_by = excluded.labeled_by, labeled_at = excluded.labeled_at
RETURNING *;

-- name: ListQualificationLabels :many
SELECT * FROM qualification_labels WHERE workspace_id = ? ORDER BY call_id;

-- name: CountObjections :one
SELECT COUNT(*) FROM objections WHERE workspace_id = ?;

-- name: CountActiveSuppressions :one
SELECT COUNT(*) FROM contact_suppressions WHERE workspace_id = ? AND lifted_at IS NULL;

-- name: CountMessageDraftsByState :many
SELECT state, COUNT(*) AS n FROM message_drafts WHERE workspace_id = ? GROUP BY state;

-- name: CountFieldOutboxByState :many
SELECT state, COUNT(*) AS n FROM crm_field_outbox WHERE workspace_id = ? GROUP BY state;

-- name: CountAlertsByState :many
SELECT state, COUNT(*) AS n FROM alerts WHERE workspace_id = ? GROUP BY state;

-- name: CountAlertsSince :one
SELECT COUNT(*) FROM alerts WHERE workspace_id = ? AND first_seen_at >= ?;

-- name: CountSignalCorrections :one
SELECT COUNT(*) FROM signal_corrections WHERE workspace_id = ?;

-- name: CountCallCorrections :one
SELECT COUNT(*) FROM call_corrections c JOIN call_sessions s ON s.id = c.session_id WHERE s.workspace_id = ?;

-- name: ListEndedCallSessions :many
SELECT * FROM call_sessions WHERE workspace_id = ? AND state = 'ended' ORDER BY id;
