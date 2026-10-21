-- name: InsertMessage :one
INSERT INTO messages (order_id, sender_id, receiver_id, content, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListMessages :many
-- Newest 200 messages, returned oldest first by the caller.
SELECT * FROM messages
WHERE order_id = $1
ORDER BY seq DESC
LIMIT 200;

-- name: MarkMessagesRead :exec
UPDATE messages SET read_at = sqlc.arg(now)::timestamptz
WHERE order_id = sqlc.arg(order_id) AND receiver_id = sqlc.arg(receiver_id) AND read_at IS NULL;

-- name: CountUnreadMessages :one
SELECT COUNT(*) FROM messages
WHERE order_id = sqlc.arg(order_id) AND receiver_id = sqlc.arg(receiver_id) AND read_at IS NULL;
