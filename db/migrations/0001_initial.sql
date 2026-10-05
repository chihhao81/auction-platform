CREATE TABLE users (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    line_user_id TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    avatar_url   TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'BANNED')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_terms (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    terms_version TEXT NOT NULL,
    accepted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, terms_version)
);

CREATE INDEX user_terms_latest_by_user_idx ON user_terms (user_id, accepted_at DESC);
