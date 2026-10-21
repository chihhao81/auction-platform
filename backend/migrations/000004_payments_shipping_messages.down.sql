DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS shipments;
DROP TABLE IF EXISTS payments;
DROP INDEX IF EXISTS idx_orders_contact_due;
ALTER TABLE orders
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS received_at,
    DROP COLUMN IF EXISTS shipped_at,
    DROP COLUMN IF EXISTS paid_at;
