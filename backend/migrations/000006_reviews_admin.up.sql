-- Phase 5: reviews, bans, admin closing of auctions.

-- An admin-closed auction is CLOSED immediately: hidden, no bids, never
-- settled into an order (settled_at is set at the same time).
ALTER TABLE auctions
    ADD COLUMN closed_at    TIMESTAMPTZ,
    ADD COLUMN closed_by    UUID REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN close_reason TEXT NOT NULL DEFAULT '';

-- Ban history. One row per ban; unbanning fills the unban columns, so past
-- bans stay visible to admins.
CREATE TABLE user_bans (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reason       TEXT        NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
    banned_by    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    banned_at    TIMESTAMPTZ NOT NULL,
    unbanned_by  UUID        REFERENCES users(id) ON DELETE RESTRICT,
    unbanned_at  TIMESTAMPTZ,
    unban_reason TEXT        NOT NULL DEFAULT '',
    CONSTRAINT chk_user_bans_unban CHECK ((unbanned_at IS NULL) = (unbanned_by IS NULL))
);

CREATE INDEX idx_user_bans_user ON user_bans(user_id, banned_at DESC);
-- At most one active ban per user.
CREATE UNIQUE INDEX uq_user_bans_active ON user_bans(user_id) WHERE unbanned_at IS NULL;

-- Two-way reviews, bound to a completed order: each party reviews the other
-- at most once per order.
CREATE TABLE reviews (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID        NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    reviewer_id UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reviewee_id UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    rating      VARCHAR(10) NOT NULL CHECK (rating IN ('POSITIVE', 'NEGATIVE')),
    comment     TEXT        NOT NULL DEFAULT '' CHECK (char_length(comment) <= 500),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reviews_order_reviewer UNIQUE (order_id, reviewer_id),
    CONSTRAINT chk_reviews_parties CHECK (reviewer_id <> reviewee_id)
);

CREATE INDEX idx_reviews_reviewee ON reviews(reviewee_id, created_at DESC);

-- A user can have at most one open report per target.
CREATE UNIQUE INDEX uq_reports_open_user ON reports(reporter_id, target_user_id)
    WHERE reporter_id IS NOT NULL AND target_type = 'USER' AND status IN ('PENDING', 'REVIEWING');
CREATE UNIQUE INDEX uq_reports_open_auction ON reports(reporter_id, target_auction_id)
    WHERE reporter_id IS NOT NULL AND target_type = 'AUCTION' AND status IN ('PENDING', 'REVIEWING');
