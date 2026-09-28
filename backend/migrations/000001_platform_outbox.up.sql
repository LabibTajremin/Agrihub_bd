-- Transactional outbox (platform). Events are written in the same transaction
-- as the state change and relayed afterwards (at-least-once).
CREATE TABLE outbox (
    id            uuid PRIMARY KEY,
    topic         text        NOT NULL,
    payload       jsonb       NOT NULL,
    actor_id      text        NOT NULL DEFAULT '',
    trace_id      text        NOT NULL DEFAULT '',
    version       integer     NOT NULL,
    occurred_at   timestamptz NOT NULL,
    dispatched_at timestamptz,
    attempts      integer     NOT NULL DEFAULT 0,
    last_error    text        NOT NULL DEFAULT ''
);

CREATE INDEX outbox_pending_idx ON outbox (occurred_at, id) WHERE dispatched_at IS NULL;
