-- weather module: cached forecasts per 0.1° cell.
CREATE TABLE weather_observations (
    cell_key   text PRIMARY KEY,
    lat_e1     integer     NOT NULL,
    lng_e1     integer     NOT NULL,
    forecast   jsonb       NOT NULL,
    fetched_at timestamptz NOT NULL
);

-- alert module: subscriptions per user and generated alerts.
CREATE TABLE alert_subscriptions (
    user_id    uuid PRIMARY KEY,
    cell_key   text        NOT NULL,
    lat        double precision NOT NULL,
    lng        double precision NOT NULL,
    kinds      text[]      NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE INDEX alert_subscriptions_cell_idx ON alert_subscriptions (cell_key);

CREATE TABLE alert_alerts (
    id              uuid PRIMARY KEY,
    user_id         uuid        NOT NULL,
    kind            text        NOT NULL,
    severity        text        NOT NULL,
    title_key       text        NOT NULL,
    body_key        text        NOT NULL,
    params          jsonb       NOT NULL,
    source_event_id uuid        NOT NULL,
    created_at      timestamptz NOT NULL,
    read_at         timestamptz,
    UNIQUE (user_id, source_event_id, kind)
);
CREATE INDEX alert_alerts_user_idx ON alert_alerts (user_id, created_at DESC);
