-- diagnosis module: scans, offline-sync operation log, LWW audit.
CREATE TABLE diagnosis_scans (
    id              uuid PRIMARY KEY,
    owner_id        uuid        NOT NULL,
    field_id        uuid,
    media_id        uuid        NOT NULL,
    crop_code       text        NOT NULL,
    phash           bigint      NOT NULL DEFAULT 0,
    status          text        NOT NULL,
    idempotency_key text        NOT NULL,
    captured_at     timestamptz NOT NULL,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    note            text        NOT NULL DEFAULT '',
    saved           boolean     NOT NULL DEFAULT false,
    diagnosis       jsonb,
    UNIQUE (owner_id, idempotency_key)
);
CREATE INDEX diagnosis_scans_owner_idx ON diagnosis_scans (owner_id, created_at DESC);

CREATE TABLE diagnosis_sync_ops (
    owner_id        uuid        NOT NULL,
    idempotency_key text        NOT NULL,
    scan_id         text        NOT NULL DEFAULT '',
    outcome         text        NOT NULL,
    error_code      text        NOT NULL DEFAULT '',
    received_at     timestamptz NOT NULL,
    PRIMARY KEY (owner_id, idempotency_key)
);

CREATE TABLE diagnosis_sync_audit (
    id          uuid PRIMARY KEY,
    scan_id     uuid        NOT NULL REFERENCES diagnosis_scans (id) ON DELETE CASCADE,
    loser       jsonb       NOT NULL,
    winner      jsonb       NOT NULL,
    received_at timestamptz NOT NULL
);
CREATE INDEX diagnosis_sync_audit_scan_idx ON diagnosis_sync_audit (scan_id);
