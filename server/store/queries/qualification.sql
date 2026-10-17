-- name: CreateCallSession :one
INSERT INTO call_sessions (workspace_id, call_id, speech_provider, state, started_at, updated_at)
VALUES (?, ?, ?, 'running', ?, ?)
RETURNING *;

-- name: GetCallSession :one
SELECT * FROM call_sessions WHERE call_id = ? AND workspace_id = ?;

-- name: GetCallSessionByID :one
SELECT * FROM call_sessions WHERE id = ? AND workspace_id = ?;

-- name: FinishCallSession :exec
UPDATE call_sessions SET state = ?, ended_by = ?, error = ?, interruptions = ?, frames_dropped = ?, transcript = ?, transcript_evidence_id = ?, ended_at = ?, updated_at = ?
WHERE id = ?;

-- name: SetCallSessionReading :exec
UPDATE call_sessions SET summary = ?, answers = ?, outcome = ?, confidence = ?, requested_human = ?, callback_at = ?, review_status = ?, review_reason = ?, updated_at = ?
WHERE id = ?;

-- name: SetCallSessionApplied :execrows
UPDATE call_sessions SET applied_key = ?, applied_version = ?, applied_at = ?, applied_detail = ?, updated_at = ?
WHERE id = ? AND applied_version = ?;

-- name: CreateCallCorrection :one
INSERT INTO call_corrections (session_id, outcome, answers, note, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListCallCorrections :many
SELECT * FROM call_corrections WHERE session_id = ? ORDER BY id;

-- name: ListCallSessions :many
SELECT * FROM call_sessions WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: CountCallSessionsByReview :many
SELECT review_status, COUNT(*) AS n FROM call_sessions WHERE workspace_id = ? AND state = 'ended' GROUP BY review_status;
