-- name: CreatePayment :one
INSERT INTO payments (order_id, merchant_trade_no, amount, created_at, updated_at)
VALUES ($1, $2, $3, $4, $4)
RETURNING *;

-- name: GetPaymentByTradeNoForUpdate :one
SELECT * FROM payments WHERE merchant_trade_no = $1 FOR UPDATE;

-- name: ListPendingPaymentTradeNos :many
SELECT merchant_trade_no FROM payments
WHERE order_id = $1 AND status = 'PENDING'
ORDER BY created_at DESC
LIMIT 5;

-- name: CountRecentPayments :one
SELECT COUNT(*) FROM payments WHERE order_id = $1 AND created_at > sqlc.arg(since)::timestamptz;

-- name: SetPaymentResult :exec
UPDATE payments
SET status = $2, provider_trade_no = $3, payment_type = $4, paid_at = $5, result = $6, updated_at = $7
WHERE id = $1;
