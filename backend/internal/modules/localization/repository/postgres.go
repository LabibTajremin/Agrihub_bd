// Package repository implements the localization store on Postgres.
package repository

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
)

// Store implements domain.Store.
type Store struct{ DB *database.DB }

const languageSQL = `SELECT l.code, l.name, l.native_name, l.is_rtl, l.is_default,
	EXISTS (SELECT 1 FROM voice_assets v WHERE v.lang_code = l.code),
	COALESCE((SELECT max(version) FROM dictionary_entries d WHERE d.lang_code = l.code), 0)
	FROM languages l`

func scanLanguage(row pgx.CollectableRow) (domain.Language, error) {
	var l domain.Language
	err := row.Scan(&l.Code, &l.Name, &l.NativeName, &l.IsRTL, &l.IsDefault, &l.HasVoice, &l.Version)
	return l, err
}

// Languages lists every language in display order.
func (s Store) Languages(ctx context.Context) ([]domain.Language, error) {
	return database.Collect(ctx, s.DB.Q(ctx), scanLanguage, languageSQL+` ORDER BY l.sort_order`)
}

// Language loads one language.
func (s Store) Language(ctx context.Context, code string) (domain.Language, error) {
	ls, err := database.Collect(ctx, s.DB.Q(ctx), scanLanguage, languageSQL+` WHERE l.code = $1`, code)
	if err == nil && len(ls) == 0 {
		err = domain.ErrLanguageNotFound
	}
	if err != nil {
		return domain.Language{}, err
	}
	return ls[0], nil
}

type entry struct {
	Key     string
	Value   string
	Version int64
}

// Entries returns entries with version > since and the highest version seen
// (since itself when nothing changed).
func (s Store) Entries(ctx context.Context, lang string, since int64) (map[string]string, int64, error) {
	rows, err := database.Collect(ctx, s.DB.Q(ctx), pgx.RowToStructByPos[entry],
		`SELECT key, value, version FROM dictionary_entries WHERE lang_code = $1 AND version > $2`, lang, since)
	out := make(map[string]string, len(rows))
	version := since
	for _, r := range rows {
		out[r.Key] = r.Value
		version = max(version, r.Version)
	}
	return out, version, err
}

// Version returns the highest entry version of lang.
func (s Store) Version(ctx context.Context, lang string) (int64, error) {
	var v int64
	err := s.DB.Q(ctx).QueryRow(ctx, `SELECT COALESCE(max(version), 0) FROM dictionary_entries WHERE lang_code = $1`, lang).Scan(&v)
	return v, err
}

func split(m map[string]string) ([]string, []string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = m[k]
	}
	return keys, vals
}

// Upsert writes changed entries only; unchanged values keep their version so
// deltas stay minimal.
func (s Store) Upsert(ctx context.Context, lang string, entries map[string]string, at time.Time) (int64, int, error) {
	keys, vals := split(entries)
	tag, err := s.DB.Q(ctx).Exec(ctx, `INSERT INTO dictionary_entries (lang_code, namespace, key, value, version, updated_at)
		SELECT $1, split_part(k, '.', 1), k, v, nextval('localization_version_seq'), $4
		FROM unnest($2::text[], $3::text[]) AS input(k, v)
		ON CONFLICT (lang_code, namespace, key) DO UPDATE
		SET value = EXCLUDED.value, version = EXCLUDED.version, updated_at = EXCLUDED.updated_at
		WHERE dictionary_entries.value IS DISTINCT FROM EXCLUDED.value`, lang, keys, vals, at)
	if err != nil {
		return 0, 0, err
	}
	v, err := s.Version(ctx, lang)
	return v, int(tag.RowsAffected()), err
}

func scanVoice(row pgx.CollectableRow) (domain.VoiceAsset, error) {
	var a domain.VoiceAsset
	err := row.Scan(&a.Key, &a.MediaID, &a.URL, &a.DurationMs, &a.Checksum, &a.Version)
	return a, err
}

// VoiceAssets returns clips with version > since, ordered by key.
func (s Store) VoiceAssets(ctx context.Context, lang string, since int64) ([]domain.VoiceAsset, error) {
	return database.Collect(ctx, s.DB.Q(ctx), scanVoice, `SELECT key, media_id, url, duration_ms, checksum, version
		FROM voice_assets WHERE lang_code = $1 AND version > $2 ORDER BY key`, lang, since)
}

// UpsertVoice writes clips whose checksum or URL changed and returns the
// language's highest voice version.
func (s Store) UpsertVoice(ctx context.Context, lang string, assets []domain.VoiceAsset, at time.Time) (int64, error) {
	n := len(assets)
	keys, media, urls, sums := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	durs := make([]int32, n)
	for i, a := range assets {
		keys[i], media[i], urls[i], sums[i], durs[i] = a.Key, a.MediaID, a.URL, a.Checksum, int32(a.DurationMs) //nolint:gosec // validated ≤ 10 min
	}
	var v int64
	err := s.DB.Q(ctx).QueryRow(ctx, `WITH up AS (
		INSERT INTO voice_assets (lang_code, key, media_id, url, duration_ms, checksum, version, updated_at)
		SELECT $1, k, m, u, d, c, nextval('localization_version_seq'), $7
		FROM unnest($2::text[], $3::text[], $4::text[], $5::int[], $6::text[]) AS input(k, m, u, d, c)
		ON CONFLICT (lang_code, key) DO UPDATE
		SET media_id = EXCLUDED.media_id, url = EXCLUDED.url, duration_ms = EXCLUDED.duration_ms,
		    checksum = EXCLUDED.checksum, version = EXCLUDED.version, updated_at = EXCLUDED.updated_at
		WHERE (voice_assets.url, voice_assets.checksum) IS DISTINCT FROM (EXCLUDED.url, EXCLUDED.checksum)
		RETURNING version)
		SELECT GREATEST(COALESCE((SELECT max(version) FROM up), 0),
		                COALESCE((SELECT max(version) FROM voice_assets WHERE lang_code = $1), 0))`,
		lang, keys, media, urls, durs, sums, at).Scan(&v)
	return v, err
}
