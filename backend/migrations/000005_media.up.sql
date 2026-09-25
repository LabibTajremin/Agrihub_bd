-- media module: blob metadata (the blobs live in object storage).
CREATE TABLE media_objects (
    id           uuid PRIMARY KEY,
    owner_id     uuid        NOT NULL,
    key          text        NOT NULL UNIQUE,
    content_type text        NOT NULL,
    size_bytes   bigint      NOT NULL,
    checksum     text        NOT NULL,
    phash        bigint      NOT NULL DEFAULT 0,
    status       text        NOT NULL,
    created_at   timestamptz NOT NULL,
    uploaded_at  timestamptz
);
CREATE INDEX media_objects_owner_idx ON media_objects (owner_id, created_at);
