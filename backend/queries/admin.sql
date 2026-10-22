-- name: AdminListUsers :many
-- q matches display name (ILIKE pattern, '' = all); status '' = all.
SELECT u.*, b.banned_at AS active_ban_at, b.reason AS active_ban_reason
FROM users u
LEFT JOIN user_bans b ON b.user_id = u.id AND b.unbanned_at IS NULL
WHERE (sqlc.arg(pattern)::text = '' OR u.display_name ILIKE sqlc.arg(pattern)::text)
  AND (sqlc.arg(status)::text = '' OR u.status = sqlc.arg(status)::text)
ORDER BY u.created_at DESC, u.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetUserForUpdate :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;

-- name: SetUserStatus :exec
UPDATE users SET status = $2, updated_at = $3 WHERE id = $1;

-- name: InsertUserBan :one
INSERT INTO user_bans (user_id, reason, banned_by, banned_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: LiftActiveBan :execrows
UPDATE user_bans
SET unbanned_by = $2, unbanned_at = $3, unban_reason = $4
WHERE user_id = $1 AND unbanned_at IS NULL;

-- name: ListUserBans :many
SELECT b.*, bu.display_name AS banned_by_name, COALESCE(uu.display_name, '') AS unbanned_by_name
FROM user_bans b
JOIN users bu ON bu.id = b.banned_by
LEFT JOIN users uu ON uu.id = b.unbanned_by
WHERE b.user_id = $1
ORDER BY b.banned_at DESC, b.unbanned_at DESC NULLS FIRST, b.id;

-- name: AdminListAuctions :many
-- Every auction including CLOSED ones; q matches title.
SELECT sqlc.embed(a), u.display_name AS seller_name
FROM auctions a
JOIN users u ON u.id = a.seller_id
WHERE sqlc.arg(pattern)::text = '' OR a.title ILIKE sqlc.arg(pattern)::text
ORDER BY a.created_at DESC, a.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CloseAuction :exec
UPDATE auctions
SET closed_at = sqlc.arg(now)::timestamptz, closed_by = sqlc.arg(closed_by), close_reason = sqlc.arg(close_reason),
    settled_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE id = sqlc.arg(id);

-- name: AdminListOrders :many
SELECT o.*, b.display_name AS buyer_name, s.display_name AS seller_name
FROM orders o
JOIN users b ON b.id = o.buyer_id
JOIN users s ON s.id = o.seller_id
WHERE sqlc.arg(status)::text = '' OR o.status = sqlc.arg(status)::text
ORDER BY o.created_at DESC, o.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListPaymentsNeedingAttention :many
-- Verified payments that were not applied and need a manual refund/check.
SELECT p.*, o.buyer_id, o.seller_id, o.status AS order_status
FROM payments p
JOIN orders o ON o.id = p.order_id
WHERE p.status IN ('DUPLICATE', 'AMOUNT_MISMATCH')
ORDER BY p.updated_at DESC
LIMIT 100;
