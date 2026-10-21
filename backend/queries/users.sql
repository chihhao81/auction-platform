-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1;

-- name: GetUserByLineID :one
SELECT * FROM users
WHERE line_user_id = $1;

-- name: UpsertUserFromLine :one
-- Creates the user on first LINE login; refreshes profile and role afterwards.
-- status is never touched here so a BAN cannot be undone by logging in.
INSERT INTO users (line_user_id, display_name, avatar_url, role)
VALUES ($1, $2, $3, $4)
ON CONFLICT (line_user_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    avatar_url   = EXCLUDED.avatar_url,
    role         = EXCLUDED.role,
    updated_at   = NOW()
RETURNING *, (xmax = 0)::boolean AS created;
