-- name: CreateRun :one
INSERT INTO runs (public_id, workspace_id, kind, occurrence_key, account_id, state, params, requested_by, created_at)
VALUES (?, ?, ?, ?, ?, 'queued', ?, ?, ?)
RETURNING *;

-- name: GetRunByOccurrence :one
SELECT * FROM runs WHERE workspace_id = ? AND kind = ? AND occurrence_key = ?;

-- name: GetRun :one
SELECT * FROM runs WHERE id = ?;

-- name: GetRunByPublicID :one
SELECT * FROM runs WHERE workspace_id = ? AND public_id = ?;

-- name: ListRecentRuns :many
SELECT * FROM runs WHERE workspace_id = ? ORDER BY created_at DESC, id DESC LIMIT ?;

-- Requeues a run that failed or was interrupted, as a new attempt.
-- name: RequeueRun :execrows
UPDATE runs
SET state = 'queued', attempt = attempt + 1, error_safe = NULL, lease_owner = NULL, lease_until = NULL,
    started_at = NULL, finished_at = NULL, requested_by = ?, created_at = ?
WHERE id = ? AND state IN ('failed', 'interrupted');

-- Takes the lease only if the run is queued, or running with an expired lease.
-- name: LeaseRun :execrows
UPDATE runs
SET state = 'running', lease_owner = sqlc.arg(owner), lease_until = sqlc.arg(lease_until),
    started_at = COALESCE(started_at, sqlc.arg(now))
WHERE id = sqlc.arg(id) AND (state = 'queued' OR (state = 'running' AND lease_until < sqlc.arg(now)));

-- name: RenewRunLease :execrows
UPDATE runs SET lease_until = ? WHERE id = ? AND lease_owner = ? AND state = 'running';

-- name: UpdateRunProgress :execrows
UPDATE runs SET progress = ?, observed_through = COALESCE(?, observed_through)
WHERE id = ? AND lease_owner = ? AND state = 'running';

-- name: FinishRun :execrows
UPDATE runs
SET state = ?, completeness = ?, error_safe = ?, finished_at = ?, lease_owner = NULL, lease_until = NULL
WHERE id = ? AND lease_owner = ? AND state = 'running';

-- Marks every running run whose lease expired as interrupted.
-- name: InterruptExpiredRuns :many
UPDATE runs
SET state = 'interrupted', lease_owner = NULL, lease_until = NULL, finished_at = ?,
    error_safe = 'the worker stopped before this run finished'
WHERE state = 'running' AND lease_until < ?
RETURNING *;

-- The caller checks the existing lease in the same transaction first.
-- name: UpsertAccountLease :exec
INSERT INTO account_leases (account_id, run_id, owner, until)
VALUES (?, ?, ?, ?)
ON CONFLICT (account_id) DO UPDATE SET run_id = excluded.run_id, owner = excluded.owner, until = excluded.until;

-- name: GetAccountLease :one
SELECT * FROM account_leases WHERE account_id = ?;

-- name: ReleaseAccountLease :exec
DELETE FROM account_leases WHERE account_id = ? AND run_id = ?;

-- name: ReleaseExpiredAccountLeases :exec
DELETE FROM account_leases WHERE until < ?;

-- name: CreateEvidence :one
INSERT INTO evidence
    (workspace_id, run_id, account_id, source, kind, source_ref, scope, class, observed_at, event_at,
     payload_version, payload, payload_hash, completeness, retain_until)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetLatestEvidence :one
SELECT * FROM evidence
WHERE workspace_id = ? AND source = ? AND kind = ? AND source_ref = ?
ORDER BY payload_version DESC LIMIT 1;

-- name: SupersedeEvidence :exec
UPDATE evidence SET superseded_by = ? WHERE id = ?;

-- name: ListEvidence :many
SELECT * FROM evidence
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source = sqlc.arg(source)
  AND kind = sqlc.arg(kind)
  AND (sqlc.narg(account_id) IS NULL OR account_id = sqlc.narg(account_id))
  AND (sqlc.narg(since) IS NULL OR observed_at >= sqlc.narg(since))
  AND superseded_by IS NULL
ORDER BY observed_at DESC, id DESC
LIMIT sqlc.arg(max);

-- name: CountEvidence :one
SELECT count(*) FROM evidence WHERE workspace_id = ? AND source = ? AND kind = ?;

-- name: ListEvidenceByRun :many
SELECT * FROM evidence WHERE run_id = ? ORDER BY id;
