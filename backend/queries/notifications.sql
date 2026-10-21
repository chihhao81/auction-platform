-- name: InsertNotification :one
INSERT INTO notifications (user_id, type, title, content, related_auction_id, related_order_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListNotifications :many
SELECT * FROM notifications
WHERE user_id = $1
ORDER BY seq DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountUnreadNotifications :one
SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = sqlc.arg(now)::timestamptz
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND read_at IS NULL;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = sqlc.arg(now)::timestamptz
WHERE user_id = sqlc.arg(user_id) AND read_at IS NULL;

-- name: ListPushOptOuts :many
SELECT type FROM notification_preferences WHERE user_id = $1 AND NOT push_enabled;

-- name: UpsertPushPreference :exec
INSERT INTO notification_preferences (user_id, type, push_enabled, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, type) DO UPDATE
SET push_enabled = EXCLUDED.push_enabled, updated_at = EXCLUDED.updated_at;

-- name: IsPushEnabled :one
SELECT COALESCE(
    (SELECT push_enabled FROM notification_preferences WHERE user_id = $1 AND type = $2),
    TRUE
)::boolean AS enabled;

-- name: ListPushSubscriptions :many
SELECT * FROM push_subscriptions WHERE user_id = $1;

-- name: UpsertPushSubscription :exec
-- An endpoint belongs to one browser; if another account logs in on the same
-- browser, the subscription moves to that account.
INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $6)
ON CONFLICT (endpoint) DO UPDATE
SET user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
    user_agent = EXCLUDED.user_agent, updated_at = EXCLUDED.updated_at;

-- name: DeleteUserPushSubscription :exec
DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2;

-- name: DeletePushSubscriptionByID :exec
DELETE FROM push_subscriptions WHERE id = $1;
