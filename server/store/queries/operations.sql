-- name: AddModelUsage :exec
INSERT INTO model_usage_days (day, model, input_tokens, output_tokens, calls, refused)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (day, model) DO UPDATE SET
    input_tokens  = input_tokens + excluded.input_tokens,
    output_tokens = output_tokens + excluded.output_tokens,
    calls         = calls + excluded.calls,
    refused       = refused + excluded.refused;

-- name: SumModelUsageDay :one
SELECT CAST(COALESCE(sum(input_tokens), 0) AS INTEGER) AS input_tokens,
       CAST(COALESCE(sum(output_tokens), 0) AS INTEGER) AS output_tokens,
       CAST(COALESCE(sum(calls), 0) AS INTEGER) AS calls,
       CAST(COALESCE(sum(refused), 0) AS INTEGER) AS refused
FROM model_usage_days WHERE day = ?;

-- name: ListModelUsageDays :many
SELECT * FROM model_usage_days WHERE day >= ? ORDER BY day DESC, model;

-- name: GetModelRelease :one
SELECT * FROM model_releases WHERE workspace_id = ? AND model = ?;

-- name: ListModelReleases :many
SELECT * FROM model_releases WHERE workspace_id = ? ORDER BY evaluated_at DESC, id DESC;

-- name: UpsertModelRelease :one
INSERT INTO model_releases (workspace_id, model, state, labels, agreements, agreement, note, evaluated_by, evaluated_at)
VALUES (?, ?, 'candidate', ?, ?, ?, ?, ?, ?)
ON CONFLICT (workspace_id, model) DO UPDATE SET
    state = 'candidate', labels = excluded.labels, agreements = excluded.agreements, agreement = excluded.agreement,
    note = excluded.note, evaluated_by = excluded.evaluated_by, evaluated_at = excluded.evaluated_at,
    decided_by = NULL, decided_at = NULL
RETURNING *;

-- name: SetModelReleaseState :exec
UPDATE model_releases SET state = ?, decided_by = ?, decided_at = ? WHERE workspace_id = ? AND model = ?;

-- name: CreateLoadMeasurement :one
INSERT INTO load_measurements (ran_at, host, cpus, go_version, duration_ms, concurrency, operations, errors, p50_ms, p95_ms, max_ms, mix, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: LatestLoadMeasurement :one
SELECT * FROM load_measurements ORDER BY ran_at DESC, id DESC LIMIT 1;
