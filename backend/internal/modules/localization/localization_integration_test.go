//go:build integration

package localization_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
)

func setup(t *testing.T, db *database.DB) (*apitest.API, *localization.Module) {
	t.Helper()
	a := apitest.New(t, db)
	m, err := localization.New(localization.Deps{Kernel: a.Kernel, CacheMaxAge: 300 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return a, m
}

func seeded(t *testing.T) (*apitest.API, *localization.Module) {
	t.Helper()
	a, m := setup(t, harness.DB(t))
	changed, err := m.SeedBundled(ctx)
	if err != nil || len(changed) != 7 || changed["bn"] == 0 {
		t.Fatal(changed, err)
	}
	return a, m
}

func TestLanguages_SevenWithArabicRTLAndBanglaDefault(t *testing.T) {
	a, _ := seeded(t)
	var out transport.Languages
	a.Do("GET", "/v1/i18n/languages", nil, "").Expect(t, 200).Decode(t, &out)
	if len(out.Languages) != 7 || out.Languages[0].Code != "bn" || !out.Languages[0].IsDefault {
		t.Fatalf("%+v", out)
	}
	for _, l := range out.Languages {
		if (l.Code == "ar") != l.IsRTL {
			t.Errorf("%s rtl=%v", l.Code, l.IsRTL)
		}
		if l.Version == 0 || l.HasVoice {
			t.Errorf("%+v", l)
		}
	}
}

func TestDictionary_SnapshotETagAnd304(t *testing.T) {
	a, _ := seeded(t)
	res := a.Do("GET", "/v1/i18n/bn", nil, "").Expect(t, 200)
	var d transport.Dictionary
	res.Decode(t, &d)
	if !d.Full || d.Entries["home.quickscan.title"] != "পাতা স্ক্যান করুন" || d.Version == 0 {
		t.Fatalf("%v %v", d.Full, d.Version)
	}
	etag := res.Header.Get("ETag")
	if etag == "" || res.Header.Get("Cache-Control") != "public, max-age=300" {
		t.Fatal(res.Header)
	}
	a.Do("GET", "/v1/i18n/bn", nil, "", "If-None-Match", etag).Expect(t, 304)
	// served from the cached snapshot the second time
	a.Do("GET", "/v1/i18n/bn", nil, "").Expect(t, 200)
	if c := a.Do("GET", "/v1/i18n/xx", nil, "").Expect(t, 404).ErrorCode(); c != "i18n.language_not_found" {
		t.Fatal(c)
	}
	a.Do("GET", "/v1/i18n/bn?since=abc", nil, "").Expect(t, 400)
	a.Do("GET", "/v1/voice/bn?since=-1", nil, "").Expect(t, 400)
}

// TestDictionary_DeltaReturnsOnlyChangedEntries proves O(changed) transfer.
func TestDictionary_DeltaReturnsOnlyChangedEntries(t *testing.T) {
	a, m := seeded(t)
	var full transport.Dictionary
	a.Do("GET", "/v1/i18n/en", nil, "").Expect(t, 200).Decode(t, &full)
	admin := a.Token("admin-1", authz.Admin)
	var up transport.UpsertResponse
	body := transport.UpsertEntriesRequest{Entries: map[string]string{"home.quickscan.title": "Scan a leaf now", "app.name": "AgriSmart"}}
	a.Do("PUT", "/v1/i18n/en/entries", body, admin).Expect(t, 200).Decode(t, &up)
	if up.Changed != 1 || up.Version <= full.Version {
		t.Fatalf("only the modified key is re-versioned: %+v", up)
	}
	var delta transport.Dictionary
	a.Do("GET", "/v1/i18n/en?since="+strconv.FormatInt(full.Version, 10), nil, "").Expect(t, 200).Decode(t, &delta)
	if delta.Full || len(delta.Entries) != 1 || delta.Entries["home.quickscan.title"] != "Scan a leaf now" || delta.Version != up.Version {
		t.Fatalf("%+v", delta)
	}
	var empty transport.Dictionary
	a.Do("GET", "/v1/i18n/en?since="+strconv.FormatInt(up.Version, 10), nil, "").Expect(t, 200).Decode(t, &empty)
	if len(empty.Entries) != 0 || empty.Version != up.Version {
		t.Fatalf("%+v", empty)
	}
	var fresh transport.Dictionary
	a.Do("GET", "/v1/i18n/en", nil, "").Expect(t, 200).Decode(t, &fresh)
	if fresh.Entries["home.quickscan.title"] != "Scan a leaf now" {
		t.Fatal("cached snapshot must be rebuilt after an update")
	}
	if changed, _ := m.SeedBundled(ctx); changed["bn"] != 0 {
		t.Fatal("re-seeding unchanged values must not bump versions", changed)
	}
	events, _ := a.Relay.Flush(ctx)
	if events.Dispatched != 1 {
		t.Fatal(events)
	}
}

func TestAdminWrites_AuthorizationAndValidation(t *testing.T) {
	a, _ := seeded(t)
	body := transport.UpsertEntriesRequest{Entries: map[string]string{"home.x": "y"}}
	a.Do("PUT", "/v1/i18n/en/entries", body, "").Expect(t, 401)
	a.Do("PUT", "/v1/i18n/en/entries", body, a.Token("f", authz.Farmer)).Expect(t, 403)
	admin := a.Token("admin", authz.Admin)
	bad := transport.UpsertEntriesRequest{Entries: map[string]string{"Bad Key": "y"}}
	if c := a.Do("PUT", "/v1/i18n/en/entries", bad, admin).Expect(t, 400).ErrorCode(); c != "i18n.invalid_entry" {
		t.Fatal(c)
	}
	a.Do("PUT", "/v1/i18n/zz/entries", body, admin).Expect(t, 404)
	a.Do("PUT", "/v1/i18n/en/entries", "{", admin).Expect(t, 400)
	a.Do("PUT", "/v1/voice/en/assets", "{", admin).Expect(t, 400)
}

func TestVoice_ManifestDeltaAndHasVoice(t *testing.T) {
	a, _ := seeded(t)
	admin := a.Token("admin", authz.Admin)
	sum := "aa" + "0000000000000000000000000000000000000000000000000000000000000000"[:62]
	clip := transport.VoiceAssetInput{Key: "narration.chart.scores", URL: "https://cdn.example/bn/scores.mp3", DurationMs: 4200, Checksum: sum}
	var up transport.UpsertResponse
	a.Do("PUT", "/v1/voice/bn/assets", transport.UpsertVoiceRequest{Assets: []transport.VoiceAssetInput{clip}}, admin).Expect(t, 200).Decode(t, &up)
	res := a.Do("GET", "/v1/voice/bn", nil, "").Expect(t, 200)
	var m transport.VoiceManifest
	res.Decode(t, &m)
	if !m.Full || m.Version != up.Version || m.Clips["narration.chart.scores"].DurationMs != 4200 {
		t.Fatalf("%+v", m)
	}
	a.Do("GET", "/v1/voice/bn", nil, "", "If-None-Match", res.Header.Get("ETag")).Expect(t, 304)
	var delta transport.VoiceManifest
	a.Do("GET", "/v1/voice/bn?since="+strconv.FormatInt(up.Version, 10), nil, "").Expect(t, 200).Decode(t, &delta)
	if delta.Full || len(delta.Clips) != 0 {
		t.Fatalf("%+v", delta)
	}
	// re-registering the identical clip does not bump the version
	var again transport.UpsertResponse
	a.Do("PUT", "/v1/voice/bn/assets", transport.UpsertVoiceRequest{Assets: []transport.VoiceAssetInput{clip}}, admin).Expect(t, 200).Decode(t, &again)
	if again.Version != up.Version {
		t.Fatal("unchanged clip must keep its version", again, up)
	}
	var langs transport.Languages
	a.Do("GET", "/v1/i18n/languages", nil, "").Decode(t, &langs)
	if !langs.Languages[0].HasVoice || langs.Languages[1].HasVoice {
		t.Fatal("has_voice follows registered clips")
	}
	a.Do("GET", "/v1/voice/xx", nil, "").Expect(t, 404)
	a.Do("PUT", "/v1/voice/xx/assets", transport.UpsertVoiceRequest{Assets: []transport.VoiceAssetInput{clip}}, admin).Expect(t, 404)
	a.Do("PUT", "/v1/voice/bn/assets", transport.UpsertVoiceRequest{Assets: []transport.VoiceAssetInput{clip}}, a.Token("f", authz.Farmer)).Expect(t, 403)
}

// ---- use-case level failures ----

func deps(t *testing.T, db *database.DB) usecase.Deps {
	a := apitest.New(t, db)
	return usecase.Deps{Store: repository.Store{DB: db}, Catalog: &domain.Catalog{}, Tx: db, Clock: a.Clock, Events: a.Kernel.Events, Factory: a.Kernel.Factory()}
}

var adminCtx = authn.WithPrincipal(ctx, authn.Principal{Subject: "admin", Role: authz.Admin})

func TestUseCases_StorageFailures(t *testing.T) {
	since := int64(0)
	cases := map[string]struct {
		fault harness.Fault
		run   func(d usecase.Deps) error
	}{
		"languages": {harness.Fault{Op: "query", Match: "FROM languages l", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.ListLanguages{Deps: d}.Execute(ctx)
			return err
		}},
		"language": {harness.Fault{Op: "query", Match: "WHERE l.code", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.GetDictionary{Deps: d}.Execute(ctx, "bn", nil)
			return err
		}},
		"delta": {harness.Fault{Op: "query", Match: "FROM dictionary_entries WHERE", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.GetDictionary{Deps: d}.Execute(ctx, "bn", &since)
			return err
		}},
		"version": {harness.Fault{Op: "queryrow", Match: "max(version), 0) FROM dictionary_entries", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.GetDictionary{Deps: d}.Execute(ctx, "bn", nil)
			return err
		}},
		"snapshot": {harness.Fault{Op: "query", Match: "FROM dictionary_entries WHERE", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.GetDictionary{Deps: d}.Execute(ctx, "bn", nil)
			return err
		}},
		"voice language": {harness.Fault{Op: "query", Match: "WHERE l.code", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.GetVoiceManifest{Deps: d}.Execute(ctx, "bn", nil)
			return err
		}},
		"voice": {harness.Fault{Op: "query", Match: "FROM voice_assets WHERE", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.GetVoiceManifest{Deps: d}.Execute(ctx, "bn", nil)
			return err
		}},
		"upsert": {harness.Fault{Op: "exec", Match: "INSERT INTO dictionary_entries", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.UpsertEntries{Deps: d}.Execute(adminCtx, "bn", map[string]string{"a.b": "c"})
			return err
		}},
		"upsert version": {harness.Fault{Op: "queryrow", Match: "max(version), 0) FROM dictionary_entries", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.UpsertEntries{Deps: d}.Execute(adminCtx, "bn", map[string]string{"a.b": "c"})
			return err
		}},
		"upsert publish": {harness.Fault{Op: "exec", Match: "INSERT INTO outbox", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.UpsertEntries{Deps: d}.Execute(adminCtx, "bn", map[string]string{"a.b": "c"})
			return err
		}},
		"upsert voice": {harness.Fault{Op: "queryrow", Match: "INSERT INTO voice_assets", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.UpsertVoice{Deps: d}.Execute(adminCtx, "bn", []domain.VoiceAsset{{Key: "a.b", URL: "u", Checksum: "c", DurationMs: 1}})
			return err
		}},
		"seed": {harness.Fault{Op: "exec", Match: "INSERT INTO dictionary_entries", Err: boom}, func(d usecase.Deps) error {
			_, err := usecase.Seed{Deps: d}.Execute(ctx, map[string]map[string]string{"bn": {"a.b": "c"}})
			return err
		}},
		"seed loader": {harness.Fault{}, func(d usecase.Deps) error {
			_, err := usecase.Seed{Deps: d}.From(ctx, func() (map[string]map[string]string, error) { return nil, boom })
			return err
		}},
	}
	for name, c := range cases {
		d := deps(t, harness.FaultyDB(t, c.fault))
		if err := c.run(d); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", name, err)
		}
	}
}

func TestUseCases_Validation(t *testing.T) {
	d := deps(t, harness.DB(t))
	if _, err := (usecase.UpsertVoice{Deps: d}).Execute(adminCtx, "bn", []domain.VoiceAsset{{Key: "BAD"}}); !errors.Is(err, domain.ErrInvalidEntry) {
		t.Fatal(err)
	}
	if _, err := (usecase.UpsertVoice{Deps: d}).Execute(ctx, "bn", nil); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.Seed{Deps: d}).Execute(ctx, map[string]map[string]string{"bn": {"a.b": ""}}); !errors.Is(err, domain.ErrInvalidEntry) {
		t.Fatal(err)
	}
	if len(localization.Errors()) != 2 {
		t.Fatal()
	}
}

func TestUpsert_NoChangeAndAnonymousAndListFailure(t *testing.T) {
	a, _ := seeded(t)
	var up transport.UpsertResponse
	a.Do("PUT", "/v1/i18n/en/entries", transport.UpsertEntriesRequest{Entries: map[string]string{"app.name": "AgriSmart"}},
		a.Token("admin", authz.Admin)).Expect(t, 200).Decode(t, &up)
	if up.Changed != 0 {
		t.Fatal(up)
	}
	if res, _ := a.Relay.Flush(ctx); res.Dispatched != 0 {
		t.Fatal("no event when nothing changed")
	}
	d := deps(t, harness.DB(t))
	if _, err := (usecase.UpsertEntries{Deps: d}).Execute(ctx, "en", nil); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	f, _ := setup(t, harness.FaultyDB(t, harness.Fault{Op: "query", Match: "FROM languages", Err: boom}))
	f.Do("GET", "/v1/i18n/languages", nil, "").Expect(t, 500)
}
