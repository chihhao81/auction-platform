-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, ip, user_agent, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetActiveSessionUser :one
-- Resolves a session cookie to its user and whether that user accepted the
-- given terms version, in a single round trip.
SELECT sqlc.embed(u),
       EXISTS (
           SELECT 1 FROM user_terms t
           WHERE t.user_id = u.id AND t.terms_version = sqlc.arg(terms_version)
       ) AS terms_accepted
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg(token_hash)
  AND s.revoked_at IS NULL
  AND s.expires_at > sqlc.arg(now)::timestamptz;

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz
WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: RevokeUserSessions :exec
UPDATE sessions SET revoked_at = sqlc.arg(now)::timestamptz
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: DeleteStaleSessions :execrows
-- Housekeeping: sessions that expired or were revoked more than a week ago.
DELETE FROM sessions
WHERE expires_at < sqlc.arg(cutoff)::timestamptz
   OR revoked_at < sqlc.arg(cutoff)::timestamptz;
