-- name: HasAcceptedTerms :one
SELECT EXISTS (
    SELECT 1 FROM user_terms
    WHERE user_id = $1 AND terms_version = $2
) AS accepted;

-- name: AcceptTerms :exec
-- Idempotent: re-accepting keeps the original accepted_at.
INSERT INTO user_terms (user_id, terms_version, accepted_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, terms_version) DO NOTHING;
