-- name: GetEnquiry :one
SELECT * FROM enquiries WHERE id = ? AND workspace_id = ?;

-- name: CreateObjection :one
INSERT INTO objections (workspace_id, person_id, enquiry_id, kind, text, source, evidence_id, recorded_by, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetObjection :one
SELECT * FROM objections WHERE id = ? AND workspace_id = ?;

-- name: ResolveObjection :execrows
UPDATE objections SET resolved_at = ?, resolution = ? WHERE id = ? AND resolved_at IS NULL;

-- name: ListObjectionsForPerson :many
SELECT * FROM objections WHERE person_id = ? ORDER BY recorded_at DESC, id DESC;

-- name: CreateSuppression :one
INSERT INTO contact_suppressions (workspace_id, person_id, channel, reason, source, since)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSuppression :one
SELECT * FROM contact_suppressions WHERE id = ? AND workspace_id = ?;

-- name: LiftSuppression :execrows
UPDATE contact_suppressions SET lifted_at = ?, lifted_by = ?, lift_reason = ? WHERE id = ? AND lifted_at IS NULL;

-- name: ListActiveSuppressions :many
SELECT * FROM contact_suppressions WHERE person_id = ? AND lifted_at IS NULL ORDER BY since;

-- name: ListSuppressionsForPerson :many
SELECT * FROM contact_suppressions WHERE person_id = ? ORDER BY since DESC, id DESC;

-- name: GetCurrentOutreachConfig :one
SELECT * FROM outreach_configs WHERE workspace_id = ? ORDER BY version DESC LIMIT 1;

-- name: CreateOutreachConfig :one
INSERT INTO outreach_configs (workspace_id, version, document, created_by, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: CreateFieldOutbox :one
INSERT INTO crm_field_outbox (workspace_id, person_id, account_id, external_id, field, value, state, reason, idempotency_key, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetFieldOutbox :one
SELECT * FROM crm_field_outbox WHERE id = ? AND workspace_id = ?;

-- name: GetFieldOutboxByKey :one
SELECT * FROM crm_field_outbox WHERE workspace_id = ? AND idempotency_key = ?;

-- name: ListFieldOutboxByState :many
SELECT * FROM crm_field_outbox WHERE workspace_id = ? AND state = ? ORDER BY id LIMIT ?;

-- name: ListFieldOutboxForPerson :many
SELECT * FROM crm_field_outbox WHERE person_id = ? ORDER BY id DESC;

-- name: ListFieldOutboxForTarget :many
SELECT * FROM crm_field_outbox WHERE account_id = ? AND external_id = ? AND field = ? AND state IN ('pending', 'held', 'sent') ORDER BY id DESC;

-- name: ListFieldOutbox :many
SELECT * FROM crm_field_outbox WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: SetFieldOutboxState :execrows
UPDATE crm_field_outbox SET state = ?, reason = ?, attempts = attempts + ?, conflict = ?, updated_at = ?, sent_at = COALESCE(sent_at, ?), confirmed_at = COALESCE(confirmed_at, ?)
WHERE id = ? AND state = ?;

-- name: CreateMessageDraft :one
INSERT INTO message_drafts (workspace_id, person_id, enquiry_id, channel, language, body, claims, state, reason, idempotency_key, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetMessageDraft :one
SELECT * FROM message_drafts WHERE id = ? AND workspace_id = ?;

-- name: ListMessageDraftsForPerson :many
SELECT * FROM message_drafts WHERE person_id = ? ORDER BY id DESC;

-- name: ListMessageDrafts :many
SELECT * FROM message_drafts WHERE workspace_id = ? ORDER BY id DESC LIMIT ?;

-- name: SetMessageDraftState :execrows
UPDATE message_drafts SET state = ?, reason = ?, attempts = attempts + ?, approved_by = COALESCE(?, approved_by), updated_at = ?, sent_at = COALESCE(sent_at, ?)
WHERE id = ? AND state = ?;

-- name: SetMessageDraftProviderRef :exec
UPDATE message_drafts SET provider_ref = ? WHERE id = ?;

-- name: GetLeadByExternalIDInWorkspace :one
SELECT * FROM leads WHERE workspace_id = ? AND account_id = ? AND external_id = ?;

-- name: ListLeadsForPerson :many
SELECT l.* FROM leads l
JOIN person_links k ON k.kind = 'lead' AND k.ref = CAST(l.id AS TEXT) AND k.reversed_at IS NULL
WHERE k.person_id = ? ORDER BY l.id;
