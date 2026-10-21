-- Phase 4: payments (ECPay), shipping, order messages, reports.

ALTER TABLE orders
    ADD COLUMN paid_at      TIMESTAMPTZ,
    ADD COLUMN shipped_at   TIMESTAMPTZ,
    ADD COLUMN received_at  TIMESTAMPTZ,
    ADD COLUMN completed_at TIMESTAMPTZ;

-- Jobs scan these: overdue contact, auto receive, auto complete.
CREATE INDEX idx_orders_contact_due ON orders(contact_deadline)
    WHERE status = 'AWAITING_CONTACT' AND contacted_at IS NULL AND overdue_at IS NULL;

-- One row per checkout attempt. ECPay requires a fresh MerchantTradeNo for
-- every attempt, so an order can have several PENDING payments.
CREATE TABLE payments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          UUID        NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    provider          VARCHAR(20) NOT NULL DEFAULT 'ECPAY',
    merchant_trade_no VARCHAR(20) NOT NULL UNIQUE,
    amount            BIGINT      NOT NULL CHECK (amount > 0),
    -- PENDING: waiting for the provider.
    -- PAID: verified payment applied to the order.
    -- DUPLICATE: verified payment for an order that was already paid or no
    --            longer payable; needs a manual refund.
    -- AMOUNT_MISMATCH: signed result whose amount differs; never applied.
    status            VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                      CHECK (status IN ('PENDING', 'PAID', 'DUPLICATE', 'AMOUNT_MISMATCH')),
    provider_trade_no VARCHAR(50),
    payment_type      VARCHAR(50),
    paid_at           TIMESTAMPTZ,
    -- Whitelisted, non-sensitive fields of the verified provider result.
    result            JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payments_order ON payments(order_id);
-- At most one payment can ever be applied to an order.
CREATE UNIQUE INDEX uq_payments_paid_per_order ON payments(order_id) WHERE status = 'PAID';

-- Recipient info (entered by the buyer after payment, visible only to the
-- order's seller) and the seller's logistics info. Recipient fields are
-- cleared some time after the order completes.
CREATE TABLE shipments (
    order_id          UUID PRIMARY KEY REFERENCES orders(id) ON DELETE RESTRICT,
    recipient_name    VARCHAR(50),
    recipient_phone   VARCHAR(20),
    recipient_address VARCHAR(300),
    carrier           VARCHAR(50),
    tracking_number   VARCHAR(100),
    address_set_at    TIMESTAMPTZ,
    shipped_at        TIMESTAMPTZ,
    purged_at         TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID        NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    sender_id   UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    receiver_id UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    content     TEXT        NOT NULL CHECK (char_length(content) BETWEEN 1 AND 1000),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at     TIMESTAMPTZ,
    CONSTRAINT chk_messages_parties CHECK (sender_id <> receiver_id)
);

CREATE INDEX idx_messages_order ON messages(order_id, created_at);
CREATE INDEX idx_messages_unread ON messages(receiver_id) WHERE read_at IS NULL;

-- Reports of users or auctions. reporter_id is NULL for system reports
-- (e.g. a buyer who did not contact the seller in time). Handled in Phase 5.
CREATE TABLE reports (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id       UUID         REFERENCES users(id) ON DELETE RESTRICT,
    target_type       VARCHAR(20)  NOT NULL CHECK (target_type IN ('USER', 'AUCTION')),
    target_user_id    UUID         REFERENCES users(id) ON DELETE RESTRICT,
    target_auction_id UUID         REFERENCES auctions(id) ON DELETE RESTRICT,
    order_id          UUID         REFERENCES orders(id) ON DELETE RESTRICT,
    reason            VARCHAR(50)  NOT NULL,
    description       TEXT         NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    status            VARCHAR(20)  NOT NULL DEFAULT 'PENDING'
                      CHECK (status IN ('PENDING', 'REVIEWING', 'RESOLVED', 'REJECTED')),
    resolution_note   TEXT         NOT NULL DEFAULT '',
    handled_by        UUID         REFERENCES users(id) ON DELETE RESTRICT,
    handled_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_reports_target CHECK (
        (target_type = 'USER' AND target_user_id IS NOT NULL AND target_auction_id IS NULL) OR
        (target_type = 'AUCTION' AND target_auction_id IS NOT NULL)
    )
);

CREATE INDEX idx_reports_status ON reports(status, created_at DESC);
CREATE INDEX idx_reports_reporter ON reports(reporter_id, created_at DESC);
-- A system report for the same order and reason is created at most once.
CREATE UNIQUE INDEX uq_reports_system_order_reason ON reports(order_id, reason) WHERE reporter_id IS NULL;
