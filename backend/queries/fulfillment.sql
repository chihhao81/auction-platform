-- name: GetOrderForUpdate :one
SELECT * FROM orders WHERE id = $1 FOR UPDATE;

-- name: SetOrderTimestamps :exec
-- Sets whichever timestamps are given; NULL arguments keep the current value.
UPDATE orders
SET contacted_at = COALESCE(sqlc.narg(contacted_at), contacted_at),
    paid_at      = COALESCE(sqlc.narg(paid_at), paid_at),
    shipped_at   = COALESCE(sqlc.narg(shipped_at), shipped_at),
    received_at  = COALESCE(sqlc.narg(received_at), received_at),
    completed_at = COALESCE(sqlc.narg(completed_at), completed_at)
WHERE id = sqlc.arg(id);

-- name: ClaimOverdueOrders :many
-- Atomically flags orders whose buyer missed the contact deadline, once.
UPDATE orders
SET overdue_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
WHERE status = 'AWAITING_CONTACT'
  AND contacted_at IS NULL
  AND overdue_at IS NULL
  AND contact_deadline <= sqlc.arg(now)::timestamptz
RETURNING id, buyer_id, seller_id;

-- name: ListOrdersShippedBefore :many
SELECT id FROM orders
WHERE status = 'SHIPPED' AND shipped_at <= sqlc.arg(cutoff)::timestamptz
ORDER BY shipped_at
LIMIT sqlc.arg(lim);

-- name: ListOrdersReceivedBefore :many
SELECT id FROM orders
WHERE status = 'RECEIVED' AND received_at <= sqlc.arg(cutoff)::timestamptz
ORDER BY received_at
LIMIT sqlc.arg(lim);

-- name: PurgeShipmentRecipients :execrows
-- Removes the buyer's address once it is no longer needed.
UPDATE shipments s
SET recipient_name = NULL, recipient_phone = NULL, recipient_address = NULL,
    purged_at = sqlc.arg(now)::timestamptz, updated_at = sqlc.arg(now)::timestamptz
FROM orders o
WHERE o.id = s.order_id
  AND s.purged_at IS NULL
  AND ((o.status = 'COMPLETED' AND o.completed_at <= sqlc.arg(cutoff)::timestamptz)
    OR (o.status = 'CANCELLED' AND o.updated_at <= sqlc.arg(cutoff)::timestamptz));

-- name: UpsertShipmentAddress :execrows
-- Recipient info can be changed until the seller ships.
INSERT INTO shipments (order_id, recipient_name, recipient_phone, recipient_address, address_set_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5, $5)
ON CONFLICT (order_id) DO UPDATE
SET recipient_name = EXCLUDED.recipient_name, recipient_phone = EXCLUDED.recipient_phone,
    recipient_address = EXCLUDED.recipient_address, address_set_at = EXCLUDED.address_set_at,
    updated_at = EXCLUDED.updated_at
WHERE shipments.shipped_at IS NULL;

-- name: GetShipment :one
SELECT * FROM shipments WHERE order_id = $1;

-- name: SetShipmentShipped :execrows
UPDATE shipments
SET carrier = $2, tracking_number = $3, shipped_at = $4, updated_at = $4
WHERE order_id = $1 AND shipped_at IS NULL AND recipient_address IS NOT NULL;

-- name: InsertSystemReport :exec
INSERT INTO reports (target_type, target_user_id, order_id, reason, description, created_at, updated_at)
VALUES ('USER', $1, $2, $3, $4, $5, $5)
ON CONFLICT (order_id, reason) WHERE reporter_id IS NULL DO NOTHING;
