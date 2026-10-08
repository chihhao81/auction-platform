BEGIN;

CREATE TABLE auctions (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    seller_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title             TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    description       TEXT NOT NULL CHECK (char_length(description) BETWEEN 1 AND 10000),
    starting_price    BIGINT NOT NULL DEFAULT 0 CHECK (starting_price >= 0),
    current_price     BIGINT NOT NULL DEFAULT 0 CHECK (current_price >= 0),
    min_increment     BIGINT NOT NULL DEFAULT 50 CHECK (min_increment > 0),
    max_increment     BIGINT NOT NULL DEFAULT 50 CHECK (max_increment >= min_increment),
    starts_at         TIMESTAMPTZ NOT NULL,
    ends_at           TIMESTAMPTZ NOT NULL,
    highest_bidder_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    contact_method    TEXT NOT NULL DEFAULT '' CHECK (char_length(contact_method) <= 500),
    shipping_method   TEXT NOT NULL DEFAULT '' CHECK (char_length(shipping_method) <= 500),
    shipping_fee      BIGINT NOT NULL DEFAULT 0 CHECK (shipping_fee >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);

CREATE INDEX auctions_public_window_idx ON auctions (starts_at, ends_at, id);
CREATE INDEX auctions_seller_window_idx ON auctions (seller_id, starts_at, ends_at);

CREATE TABLE auction_image_uploads (
    id         TEXT PRIMARY KEY CHECK (id ~ '^[a-f0-9]{64}$'),
    owner_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    auction_id BIGINT REFERENCES auctions(id) ON DELETE RESTRICT,
    mime_type  TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/gif')),
    byte_size  INTEGER NOT NULL CHECK (byte_size BETWEEN 1 AND 4194304),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX auction_image_uploads_owner_pending_idx ON auction_image_uploads (owner_id, created_at) WHERE auction_id IS NULL;

CREATE TABLE auction_images (
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE CASCADE,
    position   SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 3),
    upload_id  TEXT NOT NULL UNIQUE REFERENCES auction_image_uploads(id) ON DELETE RESTRICT,
    PRIMARY KEY (auction_id, position)
);

CREATE TABLE bids (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    auction_id BIGINT NOT NULL REFERENCES auctions(id) ON DELETE RESTRICT,
    bidder_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount     BIGINT NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (auction_id, amount)
);

CREATE INDEX bids_auction_history_idx ON bids (auction_id, created_at DESC, id DESC);
CREATE INDEX bids_bidder_history_idx ON bids (bidder_id, created_at DESC);

COMMIT;
