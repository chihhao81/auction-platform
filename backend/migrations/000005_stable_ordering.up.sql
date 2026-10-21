-- Rows created in the same transaction share created_at, and UUIDs are
-- random, so (created_at, id) does not give insertion order. A sequence
-- column gives messages and notifications a stable chronological order.
ALTER TABLE messages ADD COLUMN seq BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY;
ALTER TABLE notifications ADD COLUMN seq BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY;

DROP INDEX IF EXISTS idx_messages_order;
CREATE INDEX idx_messages_order ON messages(order_id, seq);
DROP INDEX IF EXISTS idx_notifications_user;
CREATE INDEX idx_notifications_user ON notifications(user_id, seq DESC);
