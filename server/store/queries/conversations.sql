-- name: CreateThread :one
INSERT INTO threads (workspace_id, person_id, enquiry_id, channel, address, state, state_reason, language, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetThread :one
SELECT * FROM threads WHERE id = ? AND workspace_id = ?;

-- name: GetThreadByAddress :one
SELECT * FROM threads WHERE workspace_id = ? AND channel = ? AND address = ?;

-- name: ListThreads :many
SELECT * FROM threads
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(state) IS NULL OR state = sqlc.narg(state))
ORDER BY updated_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: ListThreadsForPerson :many
SELECT * FROM threads WHERE person_id = ? ORDER BY updated_at DESC, id DESC;

-- name: SetThreadState :execrows
UPDATE threads SET state = ?, state_reason = ?, handoff_kind = ?, handoff_at = ?, updated_at = ?
WHERE id = ? AND state = sqlc.arg(from_state);

-- name: SetThreadStateAny :exec
UPDATE threads SET state = ?, state_reason = ?, handoff_kind = ?, handoff_at = ?, updated_at = ?
WHERE id = ?;

-- name: TouchThreadInbound :exec
UPDATE threads SET last_inbound_at = ?, language = ?, updated_at = ? WHERE id = ?;

-- name: TouchThreadOutbound :exec
UPDATE threads SET last_outbound_at = ?, updated_at = ? WHERE id = ?;

-- name: SetThreadEnquiry :exec
UPDATE threads SET enquiry_id = ?, updated_at = ? WHERE id = ?;

-- name: SetThreadReading :exec
UPDATE threads SET summary = ?, answers = ?, outcome = ?, confidence = ?, requested_human = ?, review_status = ?, review_reason = ?, read_at = ?, read_turns = ?, updated_at = ?
WHERE id = ?;

-- name: SetThreadApplied :execrows
UPDATE threads SET applied_key = ?, applied_version = ?, applied_at = ?, applied_detail = ?, updated_at = ?
WHERE id = ? AND applied_version = ?;

-- name: ListThreadsPendingReview :many
SELECT * FROM threads WHERE workspace_id = ? AND review_status = 'pending' ORDER BY updated_at LIMIT ?;

-- name: ListThreadsWaiting :many
SELECT * FROM threads WHERE workspace_id = ? AND state = 'waiting' ORDER BY handoff_at, id LIMIT ?;

-- name: CreateThreadTurn :one
INSERT INTO thread_turns (thread_id, direction, author, kind, body, state, reason, handoff, provider_ref, idempotency_key, claims, evidence_id, created_by, occurred_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetThreadTurn :one
SELECT t.* FROM thread_turns t JOIN threads c ON c.id = t.thread_id
WHERE t.id = ? AND c.workspace_id = ?;

-- name: GetThreadTurnByKey :one
SELECT * FROM thread_turns WHERE thread_id = ? AND idempotency_key = ?;

-- name: GetThreadTurnByProviderRef :one
SELECT t.* FROM thread_turns t JOIN threads c ON c.id = t.thread_id
WHERE t.provider_ref = ? AND c.workspace_id = ? ORDER BY t.id DESC LIMIT 1;

-- name: ListThreadTurns :many
SELECT * FROM thread_turns WHERE thread_id = ? ORDER BY id;

-- name: LastThreadTurn :one
SELECT * FROM thread_turns WHERE thread_id = ? AND state <> 'discarded' ORDER BY id DESC LIMIT 1;

-- name: CountThreadTurnsSince :one
SELECT COUNT(*) FROM thread_turns
WHERE thread_id = ? AND direction = 'out' AND state IN ('sent', 'delivered', 'read') AND sent_at >= ?;

-- name: SetThreadTurnState :execrows
UPDATE thread_turns SET state = sqlc.arg(state), reason = sqlc.arg(reason), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state);

-- name: ApproveThreadTurn :execrows
UPDATE thread_turns SET state = 'approved', reason = '', approved_by = sqlc.arg(approved_by), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state);

-- name: MarkThreadTurnSent :execrows
UPDATE thread_turns SET state = 'sent', reason = '', provider_ref = sqlc.arg(provider_ref), sent_at = sqlc.arg(sent_at), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state);

-- name: SetThreadTurnEvidence :exec
UPDATE thread_turns SET evidence_id = ? WHERE id = ?;

-- name: ListThreadTurnsByState :many
SELECT t.* FROM thread_turns t JOIN threads c ON c.id = t.thread_id
WHERE c.workspace_id = ? AND t.direction = 'out' AND t.state = ? ORDER BY t.id LIMIT ?;

-- name: CreateThreadCorrection :one
INSERT INTO thread_corrections (thread_id, outcome, answers, note, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListThreadCorrections :many
SELECT * FROM thread_corrections WHERE thread_id = ? ORDER BY id;

-- name: SetEnquiryProject :execrows
UPDATE enquiries SET project_id = ? WHERE id = ? AND project_id IS NULL;
