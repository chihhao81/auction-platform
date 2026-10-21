-- name: InsertAuditLog :exec
INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, metadata, ip, user_agent, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
