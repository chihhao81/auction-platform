-- name: ListUnsettledEndedAuctionIDs :many
SELECT id FROM auctions
WHERE settled_at IS NULL AND end_time <= sqlc.arg(now)::timestamptz
ORDER BY end_time
LIMIT sqlc.arg(lim);

-- name: MarkAuctionSettled :exec
UPDATE auctions SET settled_at = sqlc.arg(now)::timestamptz WHERE id = sqlc.arg(id);

-- name: ListAuctionBidderIDs :many
SELECT DISTINCT bidder_id FROM bids WHERE auction_id = $1;

-- name: SellerHasUnsettledAuctionEndingBy :one
-- True while the seller still has an auction that has not been settled and
-- ends at or before the given instant; pending wins must keep waiting.
SELECT EXISTS (
    SELECT 1 FROM auctions
    WHERE seller_id = sqlc.arg(seller_id)
      AND settled_at IS NULL
      AND end_time <= sqlc.arg(until)::timestamptz
) AS busy;

-- name: InsertOrderItem :one
INSERT INTO order_items (auction_id, buyer_id, seller_id, amount, shipping_fee, won_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: ListSellersWithPendingItems :many
SELECT DISTINCT seller_id FROM order_items WHERE order_id IS NULL LIMIT sqlc.arg(lim);

-- name: LockPendingItemsForSeller :many
SELECT * FROM order_items
WHERE seller_id = $1 AND order_id IS NULL
ORDER BY buyer_id, won_at
FOR UPDATE;

-- name: CreateOrder :one
INSERT INTO orders (buyer_id, seller_id, items_total, shipping_fee, total_amount, status,
                    contact_deadline, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, 'AWAITING_CONTACT', $6, $7, $7)
RETURNING *;

-- name: AttachItemsToOrder :exec
UPDATE order_items SET order_id = sqlc.arg(order_id)
WHERE id = ANY(sqlc.arg(ids)::uuid[]) AND order_id IS NULL;

-- name: ClaimEndingSoonAuctions :many
-- Atomically marks active auctions with bids that end within the window as
-- notified, returning them so each is notified exactly once.
UPDATE auctions
SET ending_soon_notified_at = sqlc.arg(now)::timestamptz
WHERE settled_at IS NULL
  AND ending_soon_notified_at IS NULL
  AND bid_count > 0
  AND start_time <= sqlc.arg(now)::timestamptz
  AND end_time > sqlc.arg(now)::timestamptz
  AND end_time <= sqlc.arg(until)::timestamptz
RETURNING id, title;
