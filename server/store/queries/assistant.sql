-- name: CreateConversation :one
INSERT INTO assistant_conversations (workspace_id, user_id, title, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetConversation :one
SELECT * FROM assistant_conversations WHERE id = ? AND workspace_id = ?;

-- name: ListConversations :many
SELECT * FROM assistant_conversations
WHERE workspace_id = ? AND (user_id = ? OR user_id IS NULL)
ORDER BY updated_at DESC LIMIT ?;

-- name: TouchConversation :exec
UPDATE assistant_conversations SET updated_at = ?, title = CASE WHEN title = '' THEN ? ELSE title END WHERE id = ?;

-- name: CreateAssistantMessage :one
INSERT INTO assistant_messages
    (conversation_id, role, status, content, tool_calls, tool_name, tool_input, tool_result, evidence_refs, warnings,
     model, input_tokens, output_tokens, cost_micros, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListAssistantMessages :many
SELECT * FROM assistant_messages WHERE conversation_id = ? ORDER BY id;

-- name: SumAssistantTokensSince :one
SELECT COALESCE(sum(input_tokens), 0) AS input_tokens, COALESCE(sum(output_tokens), 0) AS output_tokens
FROM assistant_messages m JOIN assistant_conversations c ON c.id = m.conversation_id
WHERE c.workspace_id = ? AND m.created_at >= ?;

-- name: SumAssistantCostSince :one
SELECT CAST(COALESCE(sum(cost_micros), 0) AS INTEGER) AS cost_micros
FROM assistant_messages m JOIN assistant_conversations c ON c.id = m.conversation_id
WHERE c.workspace_id = ? AND m.created_at >= ?;
