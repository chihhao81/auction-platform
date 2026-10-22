-- name: InsertReview :one
INSERT INTO reviews (order_id, reviewer_id, reviewee_id, rating, comment, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListOrderReviews :many
SELECT * FROM reviews WHERE order_id = $1;

-- name: ListReviewsForUser :many
SELECT r.id, r.rating, r.comment, r.created_at, r.order_id, u.display_name AS reviewer_name,
       (o.buyer_id = r.reviewer_id)::boolean AS from_buyer
FROM reviews r
JOIN users u ON u.id = r.reviewer_id
JOIN orders o ON o.id = r.order_id
WHERE r.reviewee_id = $1
ORDER BY r.created_at DESC, r.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: UserReputation :one
SELECT
    COUNT(*) FILTER (WHERE rv.rating = 'POSITIVE') AS positive,
    COUNT(*) FILTER (WHERE rv.rating = 'NEGATIVE') AS negative,
    (SELECT COUNT(*) FROM orders o
     WHERE o.status = 'COMPLETED'
       AND (o.buyer_id = sqlc.arg(user_id)::uuid OR o.seller_id = sqlc.arg(user_id)::uuid)) AS completed_orders
FROM reviews rv
WHERE rv.reviewee_id = sqlc.arg(user_id)::uuid;
