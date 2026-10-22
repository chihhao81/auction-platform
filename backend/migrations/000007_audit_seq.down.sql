DROP INDEX IF EXISTS idx_audit_logs_seq;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS seq;
