-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, metadata, ip, user_agent, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: AdminListAuditLogs :many
-- Filters are optional ('' / NULL = any).
SELECT a.id, a.actor_user_id, a.action, a.target_type, a.target_id, a.metadata, a.ip, a.user_agent, a.created_at,
       COALESCE(u.display_name, '') AS actor_name
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_user_id
WHERE (sqlc.arg(action)::text = '' OR a.action = sqlc.arg(action)::text)
  AND (sqlc.arg(target_type)::text = '' OR a.target_type = sqlc.arg(target_type)::text)
  AND (sqlc.arg(target_id)::text = '' OR a.target_id = sqlc.arg(target_id)::text)
  AND (sqlc.narg(actor_user_id)::uuid IS NULL OR a.actor_user_id = sqlc.narg(actor_user_id)::uuid)
ORDER BY a.seq DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);
