-- name: CreateInteractionEvent :one
INSERT INTO interaction_events (workspace_id, person_id, enquiry_id, source, kind, occurrence_key, confidence, identity, payload, occurred_at, received_at, evidence_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetInteractionEventByKey :one
SELECT * FROM interaction_events WHERE workspace_id = ? AND source = ? AND occurrence_key = ?;

-- name: GetInteractionEvent :one
SELECT * FROM interaction_events WHERE id = ? AND workspace_id = ?;

-- name: ListInteractionEventsForPerson :many
SELECT * FROM interaction_events WHERE person_id = ? ORDER BY occurred_at DESC, id DESC LIMIT ?;

-- name: ListUnknownInteractionEvents :many
SELECT * FROM interaction_events WHERE workspace_id = ? AND person_id IS NULL AND discarded_at IS NULL ORDER BY occurred_at DESC, id DESC LIMIT ?;

-- name: ListInteractionEventsSince :many
SELECT * FROM interaction_events WHERE workspace_id = ? AND received_at >= ? ORDER BY received_at DESC, id DESC LIMIT ?;

-- name: SetInteractionEventPerson :exec
UPDATE interaction_events SET person_id = ?, enquiry_id = ?, confidence = ? WHERE id = ?;

-- name: DiscardInteractionEvent :execrows
UPDATE interaction_events SET discarded_at = ?, discard_reason = ? WHERE id = ? AND discarded_at IS NULL;

-- name: ListPeopleWithEvents :many
SELECT DISTINCT person_id FROM interaction_events WHERE workspace_id = ? AND person_id IS NOT NULL AND occurred_at >= ?;

-- name: GetCurrentScoringRules :one
SELECT * FROM scoring_rules WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: GetScoringRulesVersion :one
SELECT * FROM scoring_rules WHERE workspace_id = ? AND version = ?;

-- name: CreateScoringRules :one
INSERT INTO scoring_rules (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateScoreAssessment :one
INSERT INTO score_assessments (workspace_id, person_id, rules_version, trigger, fit, intent, temperature, explanation, assessed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: LatestScoreAssessment :one
SELECT * FROM score_assessments WHERE person_id = ? ORDER BY assessed_at DESC, id DESC LIMIT 1;

-- name: ListScoreAssessments :many
SELECT * FROM score_assessments WHERE person_id = ? ORDER BY assessed_at DESC, id DESC LIMIT ?;

-- name: CreateSignalCorrection :one
INSERT INTO signal_corrections (workspace_id, event_id, action, from_person_id, to_person_id, reason, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListSignalCorrectionsForEvent :many
SELECT * FROM signal_corrections WHERE event_id = ? ORDER BY id;

-- name: ListLinkedLeadsForSignals :many
SELECT l.*, k.person_id FROM leads l
JOIN person_links k ON k.kind = 'lead' AND k.ref = CAST(l.id AS TEXT) AND k.reversed_at IS NULL
WHERE l.workspace_id = ? AND l.status_raw IS NOT NULL AND l.status_observed_at IS NOT NULL
ORDER BY l.id LIMIT ?;

-- name: ListLinkedSubmissionsForSignals :many
SELECT s.*, k.person_id FROM form_submissions s
JOIN person_links k ON k.kind = 'form_submission' AND k.ref = CAST(s.id AS TEXT) AND k.reversed_at IS NULL
WHERE s.workspace_id = ? AND s.test = 0
ORDER BY s.id LIMIT ?;
