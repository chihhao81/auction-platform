DROP INDEX IF EXISTS uq_reports_open_auction;
DROP INDEX IF EXISTS uq_reports_open_user;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS user_bans;
ALTER TABLE auctions
    DROP COLUMN IF EXISTS close_reason,
    DROP COLUMN IF EXISTS closed_by,
    DROP COLUMN IF EXISTS closed_at;
