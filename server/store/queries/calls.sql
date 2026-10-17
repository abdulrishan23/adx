-- name: GetCurrentCallPolicy :one
SELECT * FROM call_policies WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreateCallPolicy :one
INSERT INTO call_policies (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateCall :one
INSERT INTO calls (workspace_id, person_id, enquiry_id, occurrence_key, mode, to_number, from_number, state, reason, provider, policy_version, consent_basis, attempt, record, requested_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'reserved', ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetCall :one
SELECT * FROM calls WHERE id = ? AND workspace_id = ?;

-- name: GetCallByOccurrence :one
SELECT * FROM calls WHERE workspace_id = ? AND occurrence_key = ?;

-- name: GetCallByProviderID :one
SELECT * FROM calls WHERE provider = ? AND provider_call_id = ?;

-- name: CountActiveCalls :one
SELECT COUNT(*) FROM calls WHERE workspace_id = ? AND state IN ('reserved', 'queued', 'ringing', 'in_progress', 'unknown');

-- name: ListActiveCallsForEnquiry :many
SELECT * FROM calls WHERE enquiry_id = ? AND state IN ('reserved', 'queued', 'ringing', 'in_progress', 'unknown') ORDER BY id;

-- name: CountCallsForEnquirySince :one
SELECT COUNT(*) FROM calls WHERE enquiry_id = ? AND created_at >= ?;

-- name: ListCalls :many
SELECT * FROM calls WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: ListCallsForPerson :many
SELECT * FROM calls WHERE person_id = ? ORDER BY id DESC;

-- name: ListCallsByState :many
SELECT * FROM calls WHERE workspace_id = ? AND state = ? ORDER BY id LIMIT ?;

-- name: SetCallState :execrows
UPDATE calls SET state = ?, reason = ?, updated_at = ?, started_at = COALESCE(started_at, ?), ended_at = COALESCE(ended_at, ?), unknown_since = ?, duration_seconds = COALESCE(?, duration_seconds)
WHERE id = ? AND state = ?;

-- name: SetCallProviderID :exec
UPDATE calls SET provider_call_id = ?, updated_at = ? WHERE id = ?;

-- name: SetCallRecording :exec
UPDATE calls SET recording_ref = ?, updated_at = ? WHERE id = ?;

-- name: MarkRecordingDeleted :execrows
UPDATE calls SET recording_deleted_at = ?, updated_at = ? WHERE id = ? AND recording_deleted_at IS NULL AND recording_ref != '';

-- name: ListCallsWithRecordingsBefore :many
SELECT * FROM calls WHERE workspace_id = ? AND recording_ref != '' AND recording_deleted_at IS NULL AND ended_at IS NOT NULL AND ended_at < ? ORDER BY id LIMIT ?;

-- name: CreateCallEvent :one
INSERT INTO call_events (call_id, kind, from_state, to_state, detail, payload, event_key, received_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListCallEvents :many
SELECT * FROM call_events WHERE call_id = ? ORDER BY id;
