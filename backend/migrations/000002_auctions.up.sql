-- Phase 2: auctions, images and bids.
-- Money is stored as BIGINT whole New Taiwan Dollars (no fractional units).
-- Auction status (UPCOMING / ACTIVE / ENDED / CLOSED) is derived from
-- start_time / end_time and the current time, so no background job is needed
-- to flip it.

CREATE TABLE auctions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id         UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title             VARCHAR(100) NOT NULL,
    description       TEXT         NOT NULL CHECK (char_length(description) <= 5000),
    start_price       BIGINT       NOT NULL CHECK (start_price >= 0),
    min_increment     BIGINT       NOT NULL CHECK (min_increment >= 1),
    max_increment     BIGINT       NOT NULL,
    current_price     BIGINT       NOT NULL,
    highest_bidder_id UUID         REFERENCES users(id) ON DELETE RESTRICT,
    bid_count         INTEGER      NOT NULL DEFAULT 0 CHECK (bid_count >= 0),
    start_time        TIMESTAMPTZ  NOT NULL,
    end_time          TIMESTAMPTZ  NOT NULL,
    -- end_time before any anti-sniping extension, kept for reference.
    original_end_time TIMESTAMPTZ  NOT NULL,
    contact_info      VARCHAR(500) NOT NULL DEFAULT '',
    shipping_method   VARCHAR(100) NOT NULL,
    shipping_fee      BIGINT       NOT NULL CHECK (shipping_fee >= 0),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_auctions_increment CHECK (max_increment >= min_increment),
    CONSTRAINT chk_auctions_price CHECK (current_price >= start_price),
    CONSTRAINT chk_auctions_time CHECK (end_time > start_time AND end_time >= original_end_time),
    CONSTRAINT chk_auctions_bidder CHECK ((highest_bidder_id IS NULL) = (bid_count = 0)),
    CONSTRAINT chk_auctions_no_self_bid CHECK (highest_bidder_id IS NULL OR highest_bidder_id <> seller_id)
);

CREATE INDEX idx_auctions_end_time ON auctions(end_time);
CREATE INDEX idx_auctions_seller_end ON auctions(seller_id, end_time DESC);

CREATE TABLE auction_images (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    auction_id  UUID         NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    -- Server generated object key; never derived from the uploaded filename.
    storage_key VARCHAR(200) NOT NULL UNIQUE,
    position    SMALLINT     NOT NULL CHECK (position BETWEEN 0 AND 3),
    width       INTEGER      NOT NULL CHECK (width > 0),
    height      INTEGER      NOT NULL CHECK (height > 0),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_auction_images_position UNIQUE (auction_id, position)
);

CREATE TABLE bids (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    auction_id UUID        NOT NULL REFERENCES auctions(id) ON DELETE RESTRICT,
    bidder_id  UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Last line of defence against two bids at the same price.
    CONSTRAINT uq_bids_auction_amount UNIQUE (auction_id, amount)
);

CREATE INDEX idx_bids_auction_created ON bids(auction_id, created_at DESC);
CREATE INDEX idx_bids_bidder ON bids(bidder_id, created_at DESC);
