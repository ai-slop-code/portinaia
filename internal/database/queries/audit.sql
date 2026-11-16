-- name: AppendAuditEvent :exec
INSERT INTO audit_events (
    id, occurred_at, actor_type, actor_id, action, resource_type, resource_id,
    request_id, before_document, after_document, result
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditEvents :many
SELECT sequence, id, occurred_at, actor_type, actor_id, action, resource_type,
       resource_id, request_id, before_document, after_document, result
FROM audit_events
WHERE sequence < sqlc.arg(before_sequence)
  AND (sqlc.narg(actor_type) IS NULL OR actor_type = sqlc.narg(actor_type))
  AND (sqlc.narg(actor_id) IS NULL OR actor_id = sqlc.narg(actor_id))
  AND (sqlc.narg(action) IS NULL OR action = sqlc.narg(action))
  AND (sqlc.narg(resource_type) IS NULL OR resource_type = sqlc.narg(resource_type))
  AND (sqlc.narg(resource_id) IS NULL OR resource_id = sqlc.narg(resource_id))
  AND (sqlc.narg(from_time) IS NULL OR occurred_at >= sqlc.narg(from_time))
  AND (sqlc.narg(to_time) IS NULL OR occurred_at <= sqlc.narg(to_time))
ORDER BY sequence DESC
LIMIT sqlc.arg(page_limit);
