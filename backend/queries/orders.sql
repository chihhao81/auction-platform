-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: ListOrdersForUser :many
-- role: 'buyer' or 'seller'.
SELECT o.*,
       (SELECT COUNT(*) FROM order_items i WHERE i.order_id = o.id) AS item_count,
       cp.display_name AS counterparty_name
FROM orders o
JOIN users cp ON cp.id = CASE WHEN sqlc.arg(role)::text = 'buyer' THEN o.seller_id ELSE o.buyer_id END
WHERE (sqlc.arg(role)::text = 'buyer' AND o.buyer_id = sqlc.arg(user_id))
   OR (sqlc.arg(role)::text = 'seller' AND o.seller_id = sqlc.arg(user_id))
ORDER BY o.created_at DESC, o.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListOrderItems :many
SELECT i.auction_id, i.amount, i.shipping_fee, a.title, a.contact_info, a.shipping_method,
       COALESCE((SELECT img.storage_key FROM auction_images img
                 WHERE img.auction_id = a.id ORDER BY img.position LIMIT 1), '')::text AS cover_key
FROM order_items i
JOIN auctions a ON a.id = i.auction_id
WHERE i.order_id = $1
ORDER BY i.won_at, i.id;

-- name: GetUserPublic :one
SELECT id, display_name, avatar_url FROM users WHERE id = $1;

-- name: UpdateOrderStatus :execrows
-- Optimistic transition: only succeeds if the order is still in from_status.
UPDATE orders SET status = sqlc.arg(to_status), updated_at = sqlc.arg(now)::timestamptz
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);
