// Package usecase implements the localization interactors.
package usecase

import (
	"context"
	"strconv"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/tx"
)

// Event topics.
const (
	TopicDictionaryUpdated = "localization.dictionary.updated"
	TopicVoiceUpdated      = "localization.voice.updated"
)

// Deps are shared by the localization use cases.
type Deps struct {
	Store   domain.Store
	Catalog *domain.Catalog
	Tx      tx.Manager
	Clock   clock.Clock
	Events  eventbus.Publisher
	Factory eventbus.Factory
}

// ListLanguages returns all languages.
type ListLanguages struct{ Deps }

// Execute lists languages.
func (uc ListLanguages) Execute(ctx context.Context) ([]domain.Language, error) {
	return uc.Store.Languages(ctx)
}

// Dictionary is a full snapshot or a delta.
type Dictionary struct {
	Lang    string
	Version int64
	Full    bool
	Entries map[string]string
	ETag    string
}

// GetDictionary serves a language's dictionary: the cached snapshot (rebuilt
// when the stored version moved) or, with since, only changed entries.
type GetDictionary struct{ Deps }

// Execute returns the dictionary for lang.
func (uc GetDictionary) Execute(ctx context.Context, lang string, since *int64) (Dictionary, error) {
	if _, err := uc.Store.Language(ctx, lang); err != nil {
		return Dictionary{}, err
	}
	if since != nil {
		entries, version, err := uc.Store.Entries(ctx, lang, *since)
		if err != nil {
			return Dictionary{}, err
		}
		return Dictionary{Lang: lang, Version: version, Entries: entries, ETag: domain.ETag(version, entries)}, nil
	}
	current, err := uc.Store.Version(ctx, lang)
	if err != nil {
		return Dictionary{}, err
	}
	snap, ok := uc.Catalog.Get(lang)
	if !ok || snap.Version != current {
		entries, version, err := uc.Store.Entries(ctx, lang, 0)
		if err != nil {
			return Dictionary{}, err
		}
		snap = domain.NewSnapshot(lang, version, entries)
		uc.Catalog.Publish(snap)
	}
	return Dictionary{Lang: lang, Version: snap.Version, Full: true, Entries: snap.Entries, ETag: snap.ETag}, nil
}

// Manifest lists voice clips (full or delta).
type Manifest struct {
	Lang    string
	Version int64
	Full    bool
	Clips   []domain.VoiceAsset
	ETag    string
}

// GetVoiceManifest serves the voice manifest of lang.
type GetVoiceManifest struct{ Deps }

// Execute returns clips with version > since (all when since is nil).
func (uc GetVoiceManifest) Execute(ctx context.Context, lang string, since *int64) (Manifest, error) {
	if _, err := uc.Store.Language(ctx, lang); err != nil {
		return Manifest{}, err
	}
	var from int64
	if since != nil {
		from = *since
	}
	clips, err := uc.Store.VoiceAssets(ctx, lang, from)
	if err != nil {
		return Manifest{}, err
	}
	version := from
	tagged := make(map[string]string, len(clips))
	for _, c := range clips {
		version = max(version, c.Version)
		tagged[c.Key] = c.URL + "#" + c.Checksum + "#" + strconv.Itoa(c.DurationMs)
	}
	return Manifest{Lang: lang, Version: version, Full: since == nil, Clips: clips, ETag: domain.ETag(version, tagged)}, nil
}

// UpsertResult reports the new version and how many keys changed.
type UpsertResult struct {
	Version int64
	Changed int
}

func validateEntries(entries map[string]string) error {
	for k, v := range entries {
		if !domain.ValidKey(k) || v == "" {
			return domain.ErrInvalidEntry.WithField(k, "invalid key or empty value")
		}
	}
	return nil
}

// UpsertEntries edits a dictionary (admin, dictionary:write).
type UpsertEntries struct{ Deps }

// Execute validates and writes entries, publishing an event when anything changed.
func (uc UpsertEntries) Execute(ctx context.Context, lang string, entries map[string]string) (UpsertResult, error) {
	p, err := authn.Require(ctx, authz.DictionaryWrite)
	if err != nil {
		return UpsertResult{}, err
	}
	if err := validateEntries(entries); err != nil {
		return UpsertResult{}, err
	}
	var res UpsertResult
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := uc.Store.Language(ctx, lang); err != nil {
			return err
		}
		v, changed, err := uc.Store.Upsert(ctx, lang, entries, uc.Clock.Now())
		if err != nil {
			return err
		}
		res = UpsertResult{Version: v, Changed: changed}
		return uc.publishIf(ctx, changed > 0, TopicDictionaryUpdated, p.Subject, lang, v)
	})
	return res, err
}

func (uc Deps) publishIf(ctx context.Context, cond bool, topic, actor, lang string, version int64) error {
	if !cond {
		return nil
	}
	e, err := uc.Factory.New(ctx, topic, actor, map[string]any{"lang": lang, "version": version})
	if err == nil {
		err = uc.Events.Publish(ctx, e)
	}
	return err
}

// UpsertVoice registers pre-recorded clips (admin, voice:write).
type UpsertVoice struct{ Deps }

// Execute validates and writes clips.
func (uc UpsertVoice) Execute(ctx context.Context, lang string, assets []domain.VoiceAsset) (int64, error) {
	p, err := authn.Require(ctx, authz.VoiceWrite)
	if err != nil {
		return 0, err
	}
	for _, a := range assets {
		if !domain.ValidKey(a.Key) {
			return 0, domain.ErrInvalidEntry.WithField(a.Key, "invalid key")
		}
	}
	var version int64
	err = uc.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := uc.Store.Language(ctx, lang); err != nil {
			return err
		}
		v, err := uc.Store.UpsertVoice(ctx, lang, assets, uc.Clock.Now())
		if err != nil {
			return err
		}
		version = v
		return uc.publishIf(ctx, true, TopicVoiceUpdated, p.Subject, lang, v)
	})
	return version, err
}

// Seed loads bundled dictionaries (system operation used by cmd/seed). It is
// idempotent: unchanged values are not re-versioned.
type Seed struct{ Deps }

// Execute upserts every dictionary and returns changed counts per language.
func (uc Seed) Execute(ctx context.Context, dicts map[string]map[string]string) (map[string]int, error) {
	out := map[string]int{}
	for lang, entries := range dicts {
		if err := validateEntries(entries); err != nil {
			return nil, err
		}
		_, changed, err := uc.Store.Upsert(ctx, lang, entries, uc.Clock.Now())
		if err != nil {
			return nil, err
		}
		out[lang] = changed
	}
	return out, nil
}

// From loads dictionaries with load and seeds them.
func (uc Seed) From(ctx context.Context, load func() (map[string]map[string]string, error)) (map[string]int, error) {
	dicts, err := load()
	if err != nil {
		return nil, err
	}
	return uc.Execute(ctx, dicts)
}
