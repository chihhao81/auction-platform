-- Entries written in one transaction share created_at; seq keeps their order.
-- (ALTER TABLE does not fire the append-only row trigger.)
ALTER TABLE audit_logs ADD COLUMN seq BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY;
CREATE INDEX idx_audit_logs_seq ON audit_logs(seq DESC);
