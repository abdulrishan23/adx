-- name: CreatePerson :one
INSERT INTO people (workspace_id, display_name, primary_phone, primary_email, country, language, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPerson :one
SELECT * FROM people WHERE id = ? AND workspace_id = ?;

-- name: UpdatePersonPrimary :exec
UPDATE people SET display_name = ?, primary_phone = ?, primary_email = ?, updated_at = ? WHERE id = ?;

-- name: SetPersonMerged :exec
UPDATE people SET merged_into_id = ?, merge_record = ?, updated_at = ? WHERE id = ?;

-- name: ListPeople :many
SELECT * FROM people
WHERE workspace_id = sqlc.arg(workspace_id) AND merged_into_id IS NULL
  AND (sqlc.narg(q) IS NULL OR display_name LIKE '%' || sqlc.narg(q) || '%' OR primary_phone LIKE '%' || sqlc.narg(q) || '%' OR primary_email LIKE '%' || sqlc.narg(q) || '%')
ORDER BY updated_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: CreatePersonIdentifier :exec
INSERT INTO person_identifiers (workspace_id, person_id, kind, value, class, source, evidence_id, first_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (person_id, kind, value) DO NOTHING;

-- name: ListPersonIdentifiers :many
SELECT * FROM person_identifiers WHERE person_id = ? ORDER BY kind, first_seen_at, id;

-- name: FindPeopleByIdentifier :many
SELECT DISTINCT p.* FROM people p
JOIN person_identifiers i ON i.person_id = p.id
WHERE i.workspace_id = ? AND i.kind = ? AND i.value = ? AND p.merged_into_id IS NULL
ORDER BY p.id;

-- name: MovePersonIdentifiers :exec
UPDATE person_identifiers SET person_id = ? WHERE person_id = ?;

-- name: CreateEnquiry :one
INSERT INTO enquiries (workspace_id, person_id, project_id, state, stage, source_kind, source_ref, first_contact_at, last_activity_at, created_at)
VALUES (?, ?, ?, 'open', ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: FindOpenEnquiry :one
SELECT * FROM enquiries WHERE person_id = ? AND state = 'open'
  AND ((sqlc.narg(project_id) IS NULL AND project_id IS NULL) OR project_id = sqlc.narg(project_id))
ORDER BY last_activity_at DESC, id DESC LIMIT 1;

-- name: TouchEnquiry :exec
UPDATE enquiries SET last_activity_at = ?, stage = COALESCE(?, stage) WHERE id = ?;

-- name: ListEnquiriesForPerson :many
SELECT * FROM enquiries WHERE person_id = ? ORDER BY last_activity_at DESC, id DESC;

-- name: ListEnquiries :many
SELECT * FROM enquiries
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.narg(project_id) IS NULL OR project_id = sqlc.narg(project_id))
ORDER BY last_activity_at DESC, id DESC LIMIT sqlc.arg(max);

-- name: MoveEnquiries :exec
UPDATE enquiries SET person_id = ? WHERE person_id = ?;

-- name: CreatePersonLink :one
INSERT INTO person_links (workspace_id, person_id, kind, ref, match_level, match_evidence, linked_at, linked_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetPersonLink :one
SELECT * FROM person_links WHERE id = ? AND workspace_id = ?;

-- name: GetActiveLink :one
SELECT * FROM person_links WHERE workspace_id = ? AND kind = ? AND ref = ? AND reversed_at IS NULL;

-- name: ListPersonLinks :many
SELECT * FROM person_links WHERE person_id = ? ORDER BY linked_at, id;

-- name: ReversePersonLink :execrows
UPDATE person_links SET reversed_at = ?, reversed_by = ?, reversal_reason = ? WHERE id = ? AND reversed_at IS NULL;

-- name: MovePersonLinks :exec
UPDATE person_links SET person_id = ? WHERE person_id = ? AND reversed_at IS NULL;

-- name: CreateMatchCandidate :exec
INSERT INTO match_candidates (workspace_id, person_id, candidate_id, reason, state, created_at)
VALUES (?, ?, ?, ?, 'open', ?)
ON CONFLICT (person_id, candidate_id) DO NOTHING;

-- name: GetMatchCandidate :one
SELECT * FROM match_candidates WHERE id = ? AND workspace_id = ?;

-- name: ListOpenCandidatesForPerson :many
SELECT * FROM match_candidates WHERE (person_id = sqlc.arg(person_id) OR candidate_id = sqlc.arg(person_id)) AND state = 'open' ORDER BY id;

-- name: ListOpenCandidates :many
SELECT * FROM match_candidates WHERE workspace_id = ? AND state = 'open' ORDER BY created_at DESC, id DESC LIMIT ?;

-- name: DecideMatchCandidate :execrows
UPDATE match_candidates SET state = ?, decided_by = ?, decided_at = ?, note = ? WHERE id = ? AND state = 'open';

-- name: CreatePersonAttribute :one
INSERT INTO person_attributes (workspace_id, person_id, key, value, class, source, evidence_id, observed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListPersonAttributes :many
SELECT * FROM person_attributes WHERE person_id = ? AND superseded_at IS NULL ORDER BY key, class, observed_at;

-- name: SupersedePersonAttribute :exec
UPDATE person_attributes SET superseded_at = ? WHERE person_id = ? AND key = ? AND class = ? AND superseded_at IS NULL;

-- name: MovePersonAttributes :exec
UPDATE person_attributes SET person_id = ? WHERE person_id = ?;

-- name: ListUnlinkedLeads :many
SELECT l.* FROM leads l
WHERE l.workspace_id = ? AND l.removed_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM person_links k WHERE k.kind = 'lead' AND k.ref = CAST(l.id AS TEXT) AND k.reversed_at IS NULL)
ORDER BY l.id LIMIT ?;

-- name: ListUnlinkedSubmissions :many
SELECT s.* FROM form_submissions s
WHERE s.workspace_id = ? AND s.state = 'received' AND s.test = 0
  AND NOT EXISTS (SELECT 1 FROM person_links k WHERE k.kind = 'form_submission' AND k.ref = CAST(s.id AS TEXT) AND k.reversed_at IS NULL)
ORDER BY s.id LIMIT ?;

-- name: GetLead :one
SELECT * FROM leads WHERE id = ? AND workspace_id = ?;

-- name: GetFormSubmission :one
SELECT * FROM form_submissions WHERE id = ? AND workspace_id = ?;

-- name: DeletePersonIdentifiers :exec
DELETE FROM person_identifiers WHERE person_id = ?;

-- name: SetPersonLinkPerson :exec
UPDATE person_links SET person_id = ? WHERE id = ?;

-- name: SetEnquiryPerson :exec
UPDATE enquiries SET person_id = ? WHERE id = ?;

-- name: SetAttributePerson :exec
UPDATE person_attributes SET person_id = ? WHERE id = ?;

-- name: ListAllPersonAttributes :many
SELECT * FROM person_attributes WHERE person_id = ? ORDER BY id;
