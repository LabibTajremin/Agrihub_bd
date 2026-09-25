-- identity module: users, OTP challenges, sessions (refresh-token families).
CREATE TABLE identity_users (
    id         uuid PRIMARY KEY,
    phone      text UNIQUE,
    name       text        NOT NULL DEFAULT '',
    language   text        NOT NULL DEFAULT 'bn',
    district   text        NOT NULL DEFAULT '',
    role       text        NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE identity_otp_challenges (
    id          uuid PRIMARY KEY,
    phone       text        NOT NULL,
    code_hash   text        NOT NULL,
    attempts    integer     NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL
);
CREATE INDEX identity_otp_challenges_phone_idx ON identity_otp_challenges (phone, created_at DESC);

CREATE TABLE identity_sessions (
    id         uuid PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES identity_users (id),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE TABLE identity_refresh_tokens (
    token_hash text PRIMARY KEY,
    session_id uuid        NOT NULL REFERENCES identity_sessions (id),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);
CREATE INDEX identity_refresh_tokens_session_idx ON identity_refresh_tokens (session_id);
