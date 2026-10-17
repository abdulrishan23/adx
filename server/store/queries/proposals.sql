-- name: CreateBudgetEnvelope :one
INSERT INTO budget_envelopes (workspace_id, plan_id, plan_version, channel, account_id, currency, amount_minor, opened_by, opened_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBudgetEnvelope :one
SELECT * FROM budget_envelopes WHERE id = ? AND workspace_id = ?;

-- name: ListBudgetEnvelopes :many
SELECT * FROM budget_envelopes WHERE workspace_id = ? ORDER BY plan_id, plan_version, channel;

-- name: ListBudgetEnvelopesForPlan :many
SELECT * FROM budget_envelopes WHERE plan_id = ? AND plan_version = ? ORDER BY channel;

-- name: SumActiveReservations :one
SELECT COALESCE(SUM(amount_minor), 0) FROM budget_reservations WHERE envelope_id = ? AND state IN ('reserved', 'consumed');

-- name: SumReservationsByState :one
SELECT COALESCE(SUM(amount_minor), 0) FROM budget_reservations WHERE envelope_id = ? AND state = ?;

-- name: CreateBudgetReservation :one
INSERT INTO budget_reservations (envelope_id, proposal_id, amount_minor, state, created_at, updated_at)
VALUES (?, ?, ?, 'reserved', ?, ?)
RETURNING *;

-- name: GetBudgetReservation :one
SELECT * FROM budget_reservations WHERE id = ?;

-- name: SetBudgetReservationState :exec
UPDATE budget_reservations SET state = ?, updated_at = ? WHERE id = ?;

-- name: CreateProposal :one
INSERT INTO proposals (workspace_id, account_id, action, target_kind, target_ref, target_version, expected_current, proposed, reason, evidence_refs, amount_minor, currency, envelope_id, policy_version, policy_outcome, policy_reason, state, idempotency_key, proposed_by, proposed_via, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetProposal :one
SELECT * FROM proposals WHERE id = ? AND workspace_id = ?;

-- name: GetProposalByKey :one
SELECT * FROM proposals WHERE workspace_id = ? AND idempotency_key = ?;

-- name: ListProposals :many
SELECT * FROM proposals
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(state) IS NULL OR state = sqlc.narg(state))
ORDER BY created_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: ListLiveProposalsForTarget :many
SELECT * FROM proposals WHERE workspace_id = ? AND target_kind = ? AND target_ref = ? AND state IN ('proposed', 'approved') ORDER BY id;

-- name: SetProposalState :execrows
UPDATE proposals SET state = sqlc.arg(to_state), updated_at = sqlc.arg(updated_at) WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state);

-- name: SetProposalStateAny :exec
UPDATE proposals SET state = ?, updated_at = ? WHERE id = ?;

-- name: CreateApproval :one
INSERT INTO approvals (workspace_id, proposal_id, target_version, decision, policy_version, note, decided_by, decided_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListApprovals :many
SELECT * FROM approvals WHERE proposal_id = ? ORDER BY decided_at, id;

-- name: CreateExecutionIntent :one
INSERT INTO execution_intents (workspace_id, proposal_id, account_id, idempotency_key, reservation_id, state, authorized_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetExecutionIntent :one
SELECT * FROM execution_intents WHERE id = ? AND workspace_id = ?;

-- name: GetExecutionIntentByKey :one
SELECT * FROM execution_intents WHERE workspace_id = ? AND idempotency_key = ?;

-- name: GetExecutionIntentForProposal :one
SELECT * FROM execution_intents WHERE proposal_id = ? ORDER BY id DESC LIMIT 1;

-- name: ListExecutionIntents :many
SELECT * FROM execution_intents
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(state) IS NULL OR state = sqlc.narg(state))
ORDER BY updated_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: ListDispatchableIntents :many
SELECT * FROM execution_intents WHERE state IN ('reserved', 'uncertain', 'reconciling') AND (reconcile_after IS NULL OR reconcile_after <= ?) ORDER BY id LIMIT ?;

-- name: UpdateExecutionIntent :execrows
UPDATE execution_intents
SET state = sqlc.arg(to_state), attempts = attempts + sqlc.arg(attempt_delta), provider_refs = sqlc.arg(provider_refs), last_error_safe = sqlc.arg(last_error_safe),
    reconcile_after = sqlc.arg(reconcile_after), updated_at = sqlc.arg(updated_at),
    dispatched_at = COALESCE(sqlc.arg(dispatched_at), dispatched_at), confirmed_at = COALESCE(sqlc.arg(confirmed_at), confirmed_at)
WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state);

-- name: CreateExecutionEvent :one
INSERT INTO execution_events (intent_id, attempt, from_state, to_state, detail, by_user, at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListExecutionEvents :many
SELECT * FROM execution_events WHERE intent_id = ? ORDER BY id;

-- name: ListIntentsForPlanVersion :many
SELECT i.* FROM execution_intents i
JOIN proposals p ON p.id = i.proposal_id
JOIN budget_envelopes e ON e.id = p.envelope_id
WHERE i.workspace_id = ? AND e.plan_id = ? AND e.plan_version = ?
ORDER BY i.id;

-- name: GetExecutionIntentAny :one
SELECT * FROM execution_intents WHERE id = ?;

-- name: ListIntentsByStateAll :many
SELECT * FROM execution_intents WHERE state = ? ORDER BY updated_at, id LIMIT ?;
