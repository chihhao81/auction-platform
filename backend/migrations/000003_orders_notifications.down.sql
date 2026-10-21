DROP TABLE IF EXISTS push_subscriptions;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP INDEX IF EXISTS idx_auctions_unsettled;
ALTER TABLE auctions
    DROP COLUMN IF EXISTS ending_soon_notified_at,
    DROP COLUMN IF EXISTS settled_at;
