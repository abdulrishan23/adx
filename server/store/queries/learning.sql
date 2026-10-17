-- name: CreateLearningRecord :one
INSERT INTO learning_records (workspace_id, recommendation_ref, source, subject_kind, subject_ref, recommended, decision, window_days, created_at)
VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?)
RETURNING *;

-- name: GetLearningRecord :one
SELECT * FROM learning_records WHERE id = ? AND workspace_id = ?;

-- name: GetLearningRecordByRef :one
SELECT * FROM learning_records WHERE workspace_id = ? AND recommendation_ref = ?;

-- name: ListLearningRecords :many
SELECT * FROM learning_records WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: ListActedRecordsForSubject :many
SELECT * FROM learning_records WHERE workspace_id = ? AND subject_kind = ? AND subject_ref = ? AND action_at IS NOT NULL ORDER BY action_at DESC;

-- name: DecideLearningRecord :execrows
UPDATE learning_records SET decision = ?, decision_note = ?, decided_by = ?, decided_at = ?, action_ref = ?, action_at = ? WHERE id = ? AND decision = 'pending';

-- name: CreateLearningOutcome :one
INSERT INTO learning_outcomes (workspace_id, record_id, subject_kind, subject_ref, metric, value, unit, class, observed_at, evidence_id, note, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListOutcomesForRecord :many
SELECT * FROM learning_outcomes WHERE record_id = ? ORDER BY observed_at;

-- name: ListUnattachedOutcomes :many
SELECT * FROM learning_outcomes WHERE workspace_id = ? AND record_id IS NULL ORDER BY id DESC LIMIT ?;

-- name: CountLearningDecisions :many
SELECT decision, COUNT(*) AS n FROM learning_records WHERE workspace_id = ? GROUP BY decision;

-- name: CreateInsight :one
INSERT INTO insights (workspace_id, lineage, version, statement, scope, class, causal, support, source, control_evidence_id, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInsight :one
SELECT * FROM insights WHERE id = ? AND workspace_id = ?;

-- name: NextInsightLineage :one
SELECT COALESCE(MAX(lineage), 0) + 1 FROM insights WHERE workspace_id = ?;

-- name: ListInsights :many
SELECT * FROM insights WHERE workspace_id = ? ORDER BY lineage, version DESC;

-- name: ListInsightsByClass :many
SELECT * FROM insights WHERE workspace_id = ? AND class = ? AND retired_at IS NULL ORDER BY lineage;

-- name: ListInsightLineage :many
SELECT * FROM insights WHERE workspace_id = ? AND lineage = ? ORDER BY version;

-- name: PromoteInsight :execrows
UPDATE insights SET class = ?, causal = ?, control_evidence_id = ?, promoted_by = ?, promoted_at = ? WHERE id = ? AND class = ? AND retired_at IS NULL;

-- name: RetireInsight :execrows
UPDATE insights SET class = 'retired', retired_at = ? WHERE id = ? AND retired_at IS NULL;
