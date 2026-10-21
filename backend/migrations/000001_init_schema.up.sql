-- Phase 1: users, terms acceptance, sessions, audit logs.
-- All timestamps are TIMESTAMPTZ and written in UTC by the application.
-- Requires PostgreSQL 13+ (gen_random_uuid() is built in).

CREATE TABLE users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    line_user_id VARCHAR(64)  NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    avatar_url   TEXT         NOT NULL DEFAULT '',
    role         VARCHAR(20)  NOT NULL DEFAULT 'USER'   CHECK (role IN ('USER', 'ADMIN')),
    status       VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'BANNED')),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- One row per accepted terms version. Rows are never updated, so accepted_at
-- keeps the original acceptance time.
CREATE TABLE user_terms (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    terms_version VARCHAR(50) NOT NULL,
    accepted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_terms UNIQUE (user_id, terms_version)
);

-- Server-side sessions. Only the SHA-256 hash of the cookie token is stored,
-- so a database leak does not leak usable session tokens.
CREATE TABLE sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash BYTEA       NOT NULL UNIQUE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip         VARCHAR(64) NOT NULL DEFAULT '',
    user_agent TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID         REFERENCES users(id) ON DELETE RESTRICT,
    action        VARCHAR(100) NOT NULL,
    target_type   VARCHAR(50)  NOT NULL,
    target_id     VARCHAR(100) NOT NULL DEFAULT '',
    metadata      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    ip            VARCHAR(64)  NOT NULL DEFAULT '',
    user_agent    TEXT         NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_user_id, created_at DESC);
CREATE INDEX idx_audit_logs_target ON audit_logs(target_type, target_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);

-- Audit logs are append-only: reject UPDATE and DELETE at the database level.
CREATE FUNCTION audit_logs_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_logs_append_only
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_append_only();
