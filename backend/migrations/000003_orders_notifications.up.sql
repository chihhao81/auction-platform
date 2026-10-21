-- Phase 3: settlement, orders, notifications and Web Push subscriptions.

-- settled_at: set once by the scheduler when an ended auction is processed.
-- ending_soon_notified_at: the "ending soon" notification was sent.
ALTER TABLE auctions
    ADD COLUMN settled_at TIMESTAMPTZ,
    ADD COLUMN ending_soon_notified_at TIMESTAMPTZ;

CREATE INDEX idx_auctions_unsettled ON auctions(seller_id, end_time) WHERE settled_at IS NULL;

CREATE TABLE orders (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    buyer_id         UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    seller_id        UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    items_total      BIGINT      NOT NULL CHECK (items_total >= 0),
    shipping_fee     BIGINT      NOT NULL CHECK (shipping_fee >= 0),
    total_amount     BIGINT      NOT NULL,
    status           VARCHAR(30) NOT NULL CHECK (status IN (
                         'AWAITING_CONTACT', 'AWAITING_PAYMENT', 'PAID', 'AWAITING_SHIPMENT',
                         'SHIPPED', 'RECEIVED', 'COMPLETED', 'CANCELLED', 'DISPUTED')),
    -- The buyer must contact the seller (via order messages) before this.
    contact_deadline TIMESTAMPTZ NOT NULL,
    contacted_at     TIMESTAMPTZ,
    overdue_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_orders_total CHECK (total_amount = items_total + shipping_fee),
    CONSTRAINT chk_orders_parties CHECK (buyer_id <> seller_id)
);

CREATE INDEX idx_orders_buyer ON orders(buyer_id, created_at DESC);
CREATE INDEX idx_orders_seller ON orders(seller_id, created_at DESC);
CREATE INDEX idx_orders_status ON orders(status);

-- One row per won auction (the "得標紀錄"). order_id stays NULL while the
-- win waits to be merged with the seller's other auctions ending soon.
CREATE TABLE order_items (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    auction_id   UUID        NOT NULL UNIQUE REFERENCES auctions(id) ON DELETE RESTRICT,
    buyer_id     UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    seller_id    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount       BIGINT      NOT NULL CHECK (amount > 0),
    shipping_fee BIGINT      NOT NULL CHECK (shipping_fee >= 0),
    order_id     UUID        REFERENCES orders(id) ON DELETE RESTRICT,
    won_at       TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_order_items_pending ON order_items(seller_id) WHERE order_id IS NULL;
CREATE INDEX idx_order_items_order ON order_items(order_id);

CREATE TABLE notifications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type               VARCHAR(50)  NOT NULL,
    title              VARCHAR(200) NOT NULL,
    content            TEXT         NOT NULL,
    related_auction_id UUID         REFERENCES auctions(id) ON DELETE SET NULL,
    related_order_id   UUID         REFERENCES orders(id) ON DELETE SET NULL,
    read_at            TIMESTAMPTZ,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user ON notifications(user_id, created_at DESC);
CREATE INDEX idx_notifications_unread ON notifications(user_id) WHERE read_at IS NULL;

-- Opt-outs only: a missing row means push is enabled for that type.
CREATE TABLE notification_preferences (
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         VARCHAR(50) NOT NULL,
    push_enabled BOOLEAN     NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, type)
);

-- One row per browser subscription; a user may have several devices.
CREATE TABLE push_subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT         NOT NULL UNIQUE,
    p256dh     VARCHAR(200) NOT NULL,
    auth       VARCHAR(100) NOT NULL,
    user_agent TEXT         NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_push_subscriptions_user ON push_subscriptions(user_id);
