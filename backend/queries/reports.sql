-- name: InsertUserReport :one
INSERT INTO reports (reporter_id, target_type, target_user_id, target_auction_id, reason, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
RETURNING *;

-- name: ListReportsByReporter :many
SELECT * FROM reports WHERE reporter_id = $1
ORDER BY created_at DESC, id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: AdminListReports :many
-- status '' lists all.
SELECT r.*, COALESCE(rep.display_name, '') AS reporter_name,
       COALESCE(tu.display_name, '') AS target_user_name,
       COALESCE(ta.title, '') AS target_auction_title
FROM reports r
LEFT JOIN users rep ON rep.id = r.reporter_id
LEFT JOIN users tu ON tu.id = r.target_user_id
LEFT JOIN auctions ta ON ta.id = r.target_auction_id
WHERE sqlc.arg(status)::text = '' OR r.status = sqlc.arg(status)::text
ORDER BY r.created_at DESC, r.id
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetReportForUpdate :one
SELECT * FROM reports WHERE id = $1 FOR UPDATE;

-- name: UpdateReportStatus :exec
UPDATE reports
SET status = $2, resolution_note = $3, handled_by = $4, handled_at = $5, updated_at = $5
WHERE id = $1;
