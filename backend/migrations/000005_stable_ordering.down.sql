DROP INDEX IF EXISTS idx_notifications_user;
CREATE INDEX idx_notifications_user ON notifications(user_id, created_at DESC);
DROP INDEX IF EXISTS idx_messages_order;
CREATE INDEX idx_messages_order ON messages(order_id, created_at);
ALTER TABLE notifications DROP COLUMN IF EXISTS seq;
ALTER TABLE messages DROP COLUMN IF EXISTS seq;
