-- localization module: languages, dictionary entries and voice assets.
-- Versions come from one monotonic sequence, so per-language versions are
-- monotonic too and deltas are "version > since".
CREATE SEQUENCE localization_version_seq;

CREATE TABLE languages (
    code        text PRIMARY KEY,
    name        text    NOT NULL,
    native_name text    NOT NULL,
    is_rtl      boolean NOT NULL DEFAULT false,
    is_default  boolean NOT NULL DEFAULT false,
    sort_order  integer NOT NULL
);

INSERT INTO languages (code, name, native_name, is_rtl, is_default, sort_order) VALUES
    ('bn', 'Bangla',     'বাংলা',     false, true,  1),
    ('en', 'English',    'English',   false, false, 2),
    ('hi', 'Hindi',      'हिन्दी',     false, false, 3),
    ('es', 'Spanish',    'Español',   false, false, 4),
    ('fr', 'French',     'Français',  false, false, 5),
    ('ar', 'Arabic',     'العربية',   true,  false, 6),
    ('pt', 'Portuguese', 'Português', false, false, 7);

CREATE TABLE dictionary_entries (
    lang_code  text        NOT NULL REFERENCES languages (code),
    namespace  text        NOT NULL,
    key        text        NOT NULL,
    value      text        NOT NULL,
    version    bigint      NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (lang_code, namespace, key)
);
CREATE INDEX dictionary_entries_delta_idx ON dictionary_entries (lang_code, version);

CREATE TABLE voice_assets (
    lang_code   text        NOT NULL REFERENCES languages (code),
    key         text        NOT NULL,
    media_id    text        NOT NULL DEFAULT '',
    url         text        NOT NULL,
    duration_ms integer     NOT NULL,
    checksum    text        NOT NULL,
    version     bigint      NOT NULL,
    updated_at  timestamptz NOT NULL,
    PRIMARY KEY (lang_code, key)
);
CREATE INDEX voice_assets_delta_idx ON voice_assets (lang_code, version);
