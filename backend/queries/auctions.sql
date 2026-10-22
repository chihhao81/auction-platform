-- name: LockUserStatus :one
-- Serialises auction creation per seller so the open-auction limit cannot be
-- exceeded by concurrent requests, and re-checks the ban status under lock.
SELECT status FROM users WHERE id = $1 FOR UPDATE;

-- name: CountOpenAuctionsBySeller :one
-- Open = UPCOMING or ACTIVE, i.e. not yet ended.
SELECT COUNT(*) FROM auctions
WHERE seller_id = $1 AND end_time > sqlc.arg(now)::timestamptz AND closed_at IS NULL;

-- name: CreateAuction :one
INSERT INTO auctions (
    seller_id, title, description, start_price, min_increment, max_increment,
    current_price, start_time, end_time, original_end_time,
    contact_info, shipping_method, shipping_fee, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $4, $7, $8, $8,
    $9, $10, $11, $12, $12
)
RETURNING *;

-- name: UpdateAuctionContent :one
-- Only valid while the auction has not started; the caller checks status
-- under the row lock obtained by GetAuctionForUpdate.
UPDATE auctions
SET title = $2, description = $3, start_price = $4, current_price = $4,
    min_increment = $5, max_increment = $6, start_time = $7, end_time = $8,
    original_end_time = $8, contact_info = $9, shipping_method = $10,
    shipping_fee = $11, updated_at = $12
WHERE id = $1
RETURNING *;

-- name: GetAuction :one
SELECT * FROM auctions WHERE id = $1;

-- name: GetAuctionForUpdate :one
SELECT * FROM auctions WHERE id = $1 FOR UPDATE;

-- name: ApplyBid :one
UPDATE auctions
SET current_price = $2, highest_bidder_id = $3, bid_count = bid_count + 1,
    end_time = $4, updated_at = $5
WHERE id = $1
RETURNING *;

-- name: InsertBid :one
INSERT INTO bids (auction_id, bidder_id, amount, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListBidsForAuction :many
SELECT b.id, b.amount, b.created_at, b.bidder_id, u.display_name AS bidder_name
FROM bids b
JOIN users u ON u.id = b.bidder_id
WHERE b.auction_id = $1
ORDER BY b.amount DESC
LIMIT $2;

-- name: GetAuctionWithSeller :one
SELECT sqlc.embed(a), u.display_name AS seller_name, u.avatar_url AS seller_avatar
FROM auctions a
JOIN users u ON u.id = a.seller_id
WHERE a.id = $1;

-- name: SearchAuctions :many
-- Lists auctions visible on the home page (not CLOSED, i.e. ended less than
-- 24h ago). status filter: '' (all visible), UPCOMING, ACTIVE or ENDED.
-- Ordering: active (ending soonest first), then upcoming, then ended.
SELECT sqlc.embed(a), u.display_name AS seller_name
FROM auctions a
JOIN users u ON u.id = a.seller_id
WHERE a.end_time > sqlc.arg(now)::timestamptz - INTERVAL '24 hours'
  AND a.closed_at IS NULL
  AND (sqlc.arg(pattern)::text = ''
       OR a.title ILIKE sqlc.arg(pattern)::text
       OR a.description ILIKE sqlc.arg(pattern)::text)
  AND (sqlc.arg(status)::text = ''
       OR (sqlc.arg(status)::text = 'UPCOMING' AND a.closed_at IS NULL AND a.start_time > sqlc.arg(now)::timestamptz)
       OR (sqlc.arg(status)::text = 'ACTIVE' AND a.closed_at IS NULL AND a.start_time <= sqlc.arg(now)::timestamptz AND a.end_time > sqlc.arg(now)::timestamptz)
       OR (sqlc.arg(status)::text = 'ENDED' AND (a.end_time <= sqlc.arg(now)::timestamptz OR a.closed_at IS NOT NULL)))
ORDER BY
    CASE
        WHEN a.start_time <= sqlc.arg(now)::timestamptz AND a.end_time > sqlc.arg(now)::timestamptz THEN 0
        WHEN a.start_time > sqlc.arg(now)::timestamptz THEN 1
        ELSE 2
    END,
    CASE WHEN a.end_time > sqlc.arg(now)::timestamptz THEN a.end_time END ASC,
    a.end_time DESC,
    a.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListAuctionsBySeller :many
-- status: '', UPCOMING, ACTIVE or ENDED (ENDED here includes CLOSED).
SELECT sqlc.embed(a), u.display_name AS seller_name
FROM auctions a
JOIN users u ON u.id = a.seller_id
WHERE a.seller_id = sqlc.arg(user_id)
  AND (sqlc.arg(status)::text = ''
       OR (sqlc.arg(status)::text = 'UPCOMING' AND a.closed_at IS NULL AND a.start_time > sqlc.arg(now)::timestamptz)
       OR (sqlc.arg(status)::text = 'ACTIVE' AND a.closed_at IS NULL AND a.start_time <= sqlc.arg(now)::timestamptz AND a.end_time > sqlc.arg(now)::timestamptz)
       OR (sqlc.arg(status)::text = 'ENDED' AND (a.end_time <= sqlc.arg(now)::timestamptz OR a.closed_at IS NOT NULL)))
ORDER BY a.end_time DESC, a.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: ListAuctionsByBidder :many
SELECT sqlc.embed(a), u.display_name AS seller_name
FROM auctions a
JOIN users u ON u.id = a.seller_id
WHERE EXISTS (SELECT 1 FROM bids b WHERE b.auction_id = a.id AND b.bidder_id = sqlc.arg(user_id))
  AND (sqlc.arg(status)::text = ''
       OR (sqlc.arg(status)::text = 'UPCOMING' AND a.closed_at IS NULL AND a.start_time > sqlc.arg(now)::timestamptz)
       OR (sqlc.arg(status)::text = 'ACTIVE' AND a.closed_at IS NULL AND a.start_time <= sqlc.arg(now)::timestamptz AND a.end_time > sqlc.arg(now)::timestamptz)
       OR (sqlc.arg(status)::text = 'ENDED' AND (a.end_time <= sqlc.arg(now)::timestamptz OR a.closed_at IS NOT NULL)))
ORDER BY a.end_time DESC, a.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetAuctionPrices :many
SELECT id, current_price, bid_count, start_time, end_time, closed_at
FROM auctions
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListImagesForAuctions :many
SELECT * FROM auction_images
WHERE auction_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY auction_id, position;

-- name: DeleteAuctionImages :exec
DELETE FROM auction_images WHERE auction_id = $1;

-- name: InsertAuctionImage :exec
INSERT INTO auction_images (auction_id, storage_key, position, width, height, created_at)
VALUES ($1, $2, $3, $4, $5, $6);
