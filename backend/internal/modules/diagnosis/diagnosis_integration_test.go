//go:build integration

package diagnosis_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/stub"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/cache"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
)

const (
	owner   = "00000000-0000-7000-8000-0000000000a1"
	other   = "00000000-0000-7000-8000-0000000000b2"
	missing = "00000000-0000-7000-8000-0000000000ff"
)

// fakeMedia is the consumer-side media port backed by memory.
type fakeMedia struct {
	mu      sync.Mutex
	objects map[string]domain.MediaInfo
	openErr error
	readErr bool
}

func (f *fakeMedia) add(id, ownerID string, phash uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[id] = domain.MediaInfo{OwnerID: ownerID, ContentType: "image/jpeg", PHash: phash, Uploaded: true}
}

func (f *fakeMedia) Info(_ context.Context, id string) (domain.MediaInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.objects[id]; ok {
		return o, nil
	}
	return domain.MediaInfo{}, boom
}

func (f *fakeMedia) Open(_ context.Context, id string) (io.ReadCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	if f.readErr {
		return io.NopCloser(iotest.ErrReader(boom)), nil
	}
	return io.NopCloser(bytes.NewReader([]byte("jpeg-bytes-" + id))), nil
}

// engine is a controllable DiagnosisEngine counting its calls.
type engine struct {
	mu         sync.Mutex
	calls      int
	confidence float64
	err        error
}

func (e *engine) Analyze(_ context.Context, in aiadapter.AnalyzeInput) (aiadapter.AnalyzeResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if e.err != nil {
		return aiadapter.AnalyzeResult{}, e.err
	}
	return aiadapter.AnalyzeResult{Finding: aiadapter.Finding{DiseaseCode: "rice_blast", Confidence: e.confidence}, Severity: "medium", ModelVersion: "test"}, nil
}

type env struct {
	a      *apitest.API
	media  *fakeMedia
	engine *engine
}

func setup(t *testing.T, db *database.DB, eng aiadapter.DiagnosisEngine) env {
	t.Helper()
	a := apitest.New(t, db)
	fm := &fakeMedia{objects: map[string]domain.MediaInfo{}}
	e, _ := eng.(*engine)
	m, err := diagnosis.New(diagnosis.Deps{Kernel: a.Kernel, Engine: eng, Media: fm, MinConfidence: 0.60, CacheSize: 16, MaxImageBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return env{a: a, media: fm, engine: e}
}

func newMedia(e env, ownerID string, phash uint64) string {
	id := e.a.Kernel.IDs.New()
	e.media.add(id, ownerID, phash)
	return id
}

func scanReq(mediaID, key string) transport.CreateScanRequest {
	return transport.CreateScanRequest{MediaID: mediaID, CropCode: "rice_aman", IdempotencyKey: key, CapturedAt: apitest.Epoch.Add(-time.Hour)}
}

func create(t *testing.T, e env, token string, req transport.CreateScanRequest, status int) transport.Scan {
	t.Helper()
	var s transport.Scan
	e.a.Do("POST", "/v1/scans", req, token).Expect(t, status).Decode(t, &s)
	return s
}

func TestScan_StubEngineEndToEnd(t *testing.T) {
	e := setup(t, harness.DB(t), stub.Engine{})
	tok := e.a.Token(owner, authz.Guest)
	s := create(t, e, tok, scanReq(newMedia(e, owner, 0xABCDEF1234), "k-1"), 201)
	if s.Diagnosis == nil || s.Diagnosis.ModelVersion != stub.ModelVersion || s.Diagnosis.NameKey != "disease."+s.Diagnosis.DiseaseCode+".name" {
		t.Fatalf("%+v", s)
	}
	want := string(domain.Route(s.Diagnosis.Confidence, 0.60))
	if s.Status != want || !s.CapturedAt.Equal(apitest.Epoch.Add(-time.Hour)) {
		t.Fatalf("status %s want %s", s.Status, want)
	}
	replay := create(t, e, tok, scanReq("00000000-0000-7000-8000-000000000999", "k-1"), 200)
	if replay.ID != s.ID {
		t.Fatal("idempotent replay returns the original scan")
	}
	if flushed, _ := e.a.Relay.Flush(ctx); flushed.Dispatched != 1 {
		t.Fatal(flushed)
	}
}

// TestScan_ConfidenceRouting exercises §6.9 end to end at the boundaries.
func TestScan_ConfidenceRouting(t *testing.T) {
	for conf, want := range map[float64]string{0.599: "low_confidence", 0.600: "completed", 0.601: "completed"} {
		eng := &engine{confidence: conf}
		e := setup(t, harness.DB(t), eng)
		s := create(t, e, e.a.Token(owner, authz.Farmer), scanReq(newMedia(e, owner, 0), fmt.Sprint(conf)), 201)
		if s.Status != want || len(s.Diagnosis.Plans) != 2 || s.Diagnosis.Plans[0].SafetyKey != "treatment.safety" {
			t.Errorf("%v: %+v", conf, s)
		}
	}
}

func TestScan_CacheHitSkipsEngine(t *testing.T) {
	eng := &engine{confidence: 0.9}
	e := setup(t, harness.DB(t), eng)
	tok := e.a.Token(owner, authz.Farmer)
	a := create(t, e, tok, scanReq(newMedia(e, owner, 42), "a"), 201)
	b := create(t, e, tok, scanReq(newMedia(e, owner, 42), "b"), 201)
	if eng.calls != 1 || a.ID == b.ID || b.Diagnosis.DiseaseCode != a.Diagnosis.DiseaseCode {
		t.Fatalf("re-scan of the same leaf must hit the cache: calls=%d", eng.calls)
	}
	create(t, e, tok, scanReq(newMedia(e, owner, 0), "c"), 201)
	create(t, e, tok, scanReq(newMedia(e, owner, 0), "d"), 201)
	if eng.calls != 3 {
		t.Fatal("images without a hash are never cached", eng.calls)
	}
}

func TestScan_FailureAndRetry(t *testing.T) {
	eng := &engine{confidence: 0.8, err: boom}
	e := setup(t, harness.DB(t), eng)
	tok := e.a.Token(owner, authz.Farmer)
	s := create(t, e, tok, scanReq(newMedia(e, owner, 7), "f"), 201)
	if s.Status != "failed" || s.Diagnosis != nil {
		t.Fatalf("%+v", s)
	}
	e.a.Do("POST", "/v1/scans/"+s.ID+"/retry", nil, e.a.Token(other, authz.Farmer)).Expect(t, 403)
	eng.err = nil
	var r transport.Scan
	e.a.Do("POST", "/v1/scans/"+s.ID+"/retry?lang=bn", nil, tok).Expect(t, 200).Decode(t, &r)
	if r.Status != "completed" {
		t.Fatal(r.Status)
	}
	if c := e.a.Do("POST", "/v1/scans/"+s.ID+"/retry", nil, tok).Expect(t, 409).ErrorCode(); c != "scan.invalid_transition" {
		t.Fatal(c)
	}
	e.a.Do("POST", "/v1/scans/"+missing+"/retry", nil, tok).Expect(t, 404)
	e.a.Do("POST", "/v1/scans/x/retry", nil, tok).Expect(t, 400)
	e.media.openErr = boom
	if s := create(t, e, tok, scanReq(newMedia(e, owner, 8), "g"), 201); s.Status != "failed" {
		t.Fatal("unreadable media fails the scan", s.Status)
	}
	e.media.openErr, e.media.readErr = nil, true
	if s := create(t, e, tok, scanReq(newMedia(e, owner, 9), "h"), 201); s.Status != "failed" {
		t.Fatal(s.Status)
	}
}

func TestScan_MediaValidation(t *testing.T) {
	e := setup(t, harness.DB(t), &engine{confidence: 0.9})
	tok := e.a.Token(owner, authz.Farmer)
	notMine := newMedia(e, other, 1)
	pending := e.a.Kernel.IDs.New()
	e.media.objects[pending] = domain.MediaInfo{OwnerID: owner}
	for _, id := range []string{missing, notMine, pending} {
		if c := e.a.Do("POST", "/v1/scans", scanReq(id, "v-"+id), tok).Expect(t, 400).ErrorCode(); c != "scan.invalid" {
			t.Fatal(c)
		}
	}
	e.a.Do("POST", "/v1/scans", "{", tok).Expect(t, 400)
	e.a.Do("POST", "/v1/scans", scanReq(notMine, "x"), "").Expect(t, 401)
}

func TestScan_ReadAccessListAndPagination(t *testing.T) {
	e := setup(t, harness.DB(t), &engine{confidence: 0.9})
	tok := e.a.Token(owner, authz.Farmer)
	var ids []string
	for i := range 3 {
		e.a.Clock.Advance(time.Minute)
		ids = append(ids, create(t, e, tok, scanReq(newMedia(e, owner, uint64(100+i)), fmt.Sprint("p", i)), 201).ID)
	}
	e.a.Do("GET", "/v1/scans/"+ids[0], nil, tok).Expect(t, 200)
	e.a.Do("GET", "/v1/scans/"+ids[0], nil, e.a.Token(other, authz.Farmer)).Expect(t, 403)
	e.a.Do("GET", "/v1/scans/"+ids[0], nil, e.a.Token(other, authz.FieldOfficer)).Expect(t, 200)
	e.a.Do("GET", "/v1/scans/"+missing, nil, tok).Expect(t, 404)
	e.a.Do("GET", "/v1/scans/bad", nil, tok).Expect(t, 400)

	var page transport.ScanList
	e.a.Do("GET", "/v1/scans?limit=2", nil, tok).Expect(t, 200).Decode(t, &page)
	if len(page.Scans) != 2 || page.Scans[0].ID != ids[2] || page.NextBefore == nil {
		t.Fatalf("newest first with a cursor: %+v", page)
	}
	var rest transport.ScanList
	e.a.Do("GET", "/v1/scans?limit=2&before="+url.QueryEscape(page.NextBefore.Format(time.RFC3339Nano)), nil, tok).Expect(t, 200).Decode(t, &rest)
	if len(rest.Scans) != 1 || rest.Scans[0].ID != ids[0] || rest.NextBefore != nil {
		t.Fatalf("%+v", rest)
	}
	e.a.Do("GET", "/v1/scans?owner_id="+owner, nil, e.a.Token(other, authz.Farmer)).Expect(t, 403)
	e.a.Do("GET", "/v1/scans?owner_id="+owner, nil, e.a.Token(other, authz.Agronomist)).Expect(t, 200)
	e.a.Do("GET", "/v1/scans?before=yesterday", nil, tok).Expect(t, 400)
	e.a.Do("GET", "/v1/scans?limit=0", nil, tok).Expect(t, 400)

	saved := true
	e.a.Do("PATCH", "/v1/scans/"+ids[1], transport.AnnotateRequest{Saved: &saved}, tok).Expect(t, 200)
	var log transport.ScanList
	e.a.Do("GET", "/v1/scans?saved=true", nil, tok).Expect(t, 200).Decode(t, &log)
	if len(log.Scans) != 1 || log.Scans[0].ID != ids[1] || !log.Scans[0].Saved {
		t.Fatalf("saved log: %+v", log)
	}
}

func TestAnnotate_LastWriteWinsWithAudit(t *testing.T) {
	db := harness.DB(t)
	e := setup(t, db, &engine{confidence: 0.9})
	tok := e.a.Token(owner, authz.Farmer)
	s := create(t, e, tok, scanReq(newMedia(e, owner, 5), "n"), 201)
	first, second := "sprayed on monday", "sprayed on tuesday"
	e.a.Do("PATCH", "/v1/scans/"+s.ID, transport.AnnotateRequest{Note: &first}, tok).Expect(t, 200)
	e.a.Clock.Advance(time.Minute)
	var out transport.Scan
	e.a.Do("PATCH", "/v1/scans/"+s.ID, transport.AnnotateRequest{Note: &second}, tok).Expect(t, 200).Decode(t, &out)
	if out.Note != second {
		t.Fatal(out.Note)
	}
	e.a.Do("PATCH", "/v1/scans/"+s.ID, transport.AnnotateRequest{Note: &second}, tok).Expect(t, 200)
	var n int
	var loser string
	_ = db.Q(ctx).QueryRow(ctx, `SELECT count(*), max(loser->>'note') FILTER (WHERE winner->>'note' = $2) FROM diagnosis_sync_audit WHERE scan_id = $1`, s.ID, second).Scan(&n, &loser)
	if n != 2 || loser != first {
		t.Fatalf("each overwrite is audited; no-op writes are not: n=%d loser=%q", n, loser)
	}
	e.a.Do("PATCH", "/v1/scans/"+s.ID, transport.AnnotateRequest{Note: &first}, e.a.Token(other, authz.Admin)).Expect(t, 403)
	e.a.Do("PATCH", "/v1/scans/"+missing, transport.AnnotateRequest{}, tok).Expect(t, 404)
	e.a.Do("PATCH", "/v1/scans/x", transport.AnnotateRequest{}, tok).Expect(t, 400)
	e.a.Do("PATCH", "/v1/scans/"+s.ID, "{", tok).Expect(t, 400)
}

// TestSync_DuplicateBatchIsNoOp proves offline sync idempotency (§6.7).
func TestSync_DuplicateBatchIsNoOp(t *testing.T) {
	eng := &engine{confidence: 0.9}
	e := setup(t, harness.DB(t), eng)
	tok := e.a.Token(owner, authz.Farmer)
	existing := create(t, e, tok, scanReq(newMedia(e, owner, 11), "online"), 201)
	note := "offline note"
	create1 := scanReq(newMedia(e, owner, 12), "")
	bad := scanReq(missing, "")
	batch := transport.SyncRequest{Operations: []transport.SyncOperation{
		{IdempotencyKey: "op-3", Seq: 3, Kind: "annotate_scan", ScanID: existing.ID, Annotation: &transport.AnnotateRequest{Note: &note}},
		{IdempotencyKey: "op-1", Seq: 1, Kind: "create_scan", Create: &create1},
		{IdempotencyKey: "op-2", Seq: 2, Kind: "create_scan", Create: &bad},
		{IdempotencyKey: "op-4", Seq: 4, Kind: "create_scan"},
	}}
	var res transport.SyncResponse
	e.a.Do("POST", "/v1/scans/sync", batch, tok).Expect(t, 200).Decode(t, &res)
	got := []string{}
	for _, r := range res.Results {
		got = append(got, r.IdempotencyKey+":"+r.Outcome+":"+r.ErrorCode)
	}
	want := []string{"op-1:applied:", "op-2:rejected:scan.invalid", "op-3:applied:", "op-4:rejected:scan.invalid"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("processed in seq order: %v", got)
	}
	calls := eng.calls
	var again transport.SyncResponse
	e.a.Do("POST", "/v1/scans/sync", batch, tok).Expect(t, 200).Decode(t, &again)
	for i, r := range again.Results {
		if r.Outcome != "duplicate" || r.ScanID != res.Results[i].ScanID || r.ErrorCode != res.Results[i].ErrorCode {
			t.Fatalf("replay must be a no-op: %+v", r)
		}
	}
	if eng.calls != calls {
		t.Fatal("no re-analysis on replay")
	}
	var list transport.ScanList
	e.a.Do("GET", "/v1/scans", nil, tok).Decode(t, &list)
	if len(list.Scans) != 2 {
		t.Fatal("no duplicate scans", len(list.Scans))
	}
	otherTok := e.a.Token(other, authz.Farmer)
	var forbidden transport.SyncResponse
	e.a.Do("POST", "/v1/scans/sync", transport.SyncRequest{Operations: []transport.SyncOperation{
		{IdempotencyKey: "x", Kind: "annotate_scan", ScanID: existing.ID, Annotation: &transport.AnnotateRequest{}}}}, otherTok).Expect(t, 200).Decode(t, &forbidden)
	if forbidden.Results[0].ErrorCode != "auth.forbidden" {
		t.Fatal(forbidden)
	}
	e.a.Do("POST", "/v1/scans/sync", transport.SyncRequest{}, tok).Expect(t, 400)
	e.a.Do("POST", "/v1/scans/sync", "{", tok).Expect(t, 400)
	e.a.Do("POST", "/v1/scans/sync", batch, e.a.Token(owner, authz.Guest)).Expect(t, 403)
}

// ---- use-case level failures ----

func deps(t *testing.T, db *database.DB, fm *fakeMedia, eng aiadapter.DiagnosisEngine) usecase.Deps {
	a := apitest.New(t, db)
	lru, _ := cache.NewLRU[uint64, domain.Diagnosis](8)
	return usecase.Deps{Scans: repository.Scans{DB: db}, Media: fm, Engine: eng, Cache: lru, Tx: db, Clock: a.Clock, IDs: a.Kernel.IDs,
		Events: a.Kernel.Events, Factory: a.Kernel.Factory(), MinConfidence: 0.6, MaxImageBytes: 1 << 20}
}

var farmer = authn.WithPrincipal(ctx, authn.Principal{Subject: owner, Role: authz.Farmer})

func TestUseCases_StorageFailures(t *testing.T) {
	type run func(d usecase.Deps, scanID, mediaID string) error
	createRun := func(d usecase.Deps, _, mediaID string) error {
		_, _, err := usecase.CreateScan{Deps: d}.Execute(farmer, usecase.CreateInput{MediaID: mediaID, CropCode: "rice_aman", IdempotencyKey: "new-" + mediaID})
		return err
	}
	annotateRun := func(d usecase.Deps, scanID, _ string) error {
		n := "changed"
		_, err := usecase.Annotate{Deps: d}.Execute(farmer, scanID, usecase.AnnotateInput{Note: &n})
		return err
	}
	syncRun := func(d usecase.Deps, _, mediaID string) error {
		_, err := usecase.Sync{Deps: d, Create: usecase.CreateScan{Deps: d}, Annotate: usecase.Annotate{Deps: d}}.Execute(farmer,
			[]usecase.SyncOp{{IdempotencyKey: "s1-" + mediaID, Kind: usecase.OpCreateScan, Create: &usecase.CreateInput{MediaID: mediaID, CropCode: "rice_aman"}}})
		return err
	}
	retryRun := func(d usecase.Deps, scanID, _ string) error {
		_, err := usecase.Retry{Deps: d}.Execute(farmer, scanID, "bn")
		return err
	}
	cases := []struct {
		name  string
		fault harness.Fault
		run   run
	}{
		{"get by key", harness.Fault{Op: "query", Match: "idempotency_key = $2", Err: boom}, createRun},
		{"create", harness.Fault{Op: "exec", Match: "INSERT INTO diagnosis_scans", Err: boom}, createRun},
		{"save result", harness.Fault{Op: "exec", Match: "SET status", Err: boom}, createRun},
		{"publish", harness.Fault{Op: "exec", Match: "INSERT INTO outbox", Err: boom}, createRun},
		{"annotate get", harness.Fault{Op: "query", Match: "WHERE id = $1", Err: boom}, annotateRun},
		{"annotate audit", harness.Fault{Op: "exec", Match: "INSERT INTO diagnosis_sync_audit", Err: boom}, annotateRun},
		{"annotate write", harness.Fault{Op: "exec", Match: "SET note", Err: boom}, annotateRun},
		{"sync get op", harness.Fault{Op: "queryrow", Match: "FROM diagnosis_sync_ops", Err: boom}, syncRun},
		{"sync claim", harness.Fault{Op: "exec", Match: "INSERT INTO diagnosis_sync_ops", Err: boom}, syncRun},
		{"sync internal", harness.Fault{Op: "exec", Match: "INSERT INTO diagnosis_scans", Err: boom}, syncRun},
		{"retry get", harness.Fault{Op: "query", Match: "WHERE id = $1", Err: boom}, retryRun},
	}
	for i, c := range cases {
		db := harness.DB(t)
		fm := &fakeMedia{objects: map[string]domain.MediaInfo{}}
		mediaID := fmt.Sprintf("00000000-0000-7000-8000-%012d", i+1)
		fm.add(mediaID, owner, uint64(1000+i))
		seed := deps(t, db, fm, &engine{err: boom})
		s, _, err := usecase.CreateScan{Deps: seed}.Execute(farmer, usecase.CreateInput{MediaID: mediaID, CropCode: "rice_aman", IdempotencyKey: fmt.Sprint("seed", i)})
		if err != nil {
			t.Fatal(err)
		}
		d := deps(t, harness.FaultyOver(db, c.fault), fm, &engine{confidence: 0.9})
		if err := c.run(d, s.ID, mediaID); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", c.name, err)
		}
	}
	// retry whose media lookup fails
	db := harness.DB(t)
	fm := &fakeMedia{objects: map[string]domain.MediaInfo{}}
	fm.add(missing, owner, 1)
	d := deps(t, db, fm, &engine{err: boom})
	s, _, _ := usecase.CreateScan{Deps: d}.Execute(farmer, usecase.CreateInput{MediaID: missing, CropCode: "rice_aman", IdempotencyKey: "r"})
	delete(fm.objects, missing)
	if _, err := (usecase.Retry{Deps: d}).Execute(farmer, s.ID, ""); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestUseCases_AuthAndValidation(t *testing.T) {
	d := deps(t, harness.DB(t), &fakeMedia{objects: map[string]domain.MediaInfo{}}, stub.Engine{})
	checks := map[string]error{}
	_, _, checks["create"] = usecase.CreateScan{Deps: d}.Execute(ctx, usecase.CreateInput{})
	_, checks["retry"] = usecase.Retry{Deps: d}.Execute(ctx, missing, "")
	_, checks["get"] = usecase.GetScan{Deps: d}.Execute(ctx, missing)
	_, checks["list"] = usecase.ListScans{Deps: d}.Execute(ctx, domain.ListFilter{})
	_, checks["annotate"] = usecase.Annotate{Deps: d}.Execute(ctx, missing, usecase.AnnotateInput{})
	_, checks["sync"] = usecase.Sync{Deps: d}.Execute(ctx, nil)
	for name, err := range checks {
		if !errors.Is(err, authn.ErrRequired) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := (usecase.CreateScan{Deps: d}).Execute(farmer, usecase.CreateInput{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	list, err := usecase.ListScans{Deps: d}.Execute(farmer, domain.ListFilter{Limit: 1000})
	if err != nil || len(list) != 0 {
		t.Fatal(err)
	}
	if r := usecase.SyncOutcome("k", "", boom); r.Outcome != "" {
		t.Fatal("internal errors abort the batch")
	}
	if _, err := diagnosis.New(diagnosis.Deps{CacheSize: 0}); err == nil {
		t.Fatal("cache size must be positive")
	}
	if len(diagnosis.Errors()) != 4 {
		t.Fatal()
	}
}

func TestSync_ServerErrorFailsBatch(t *testing.T) {
	e := setup(t, harness.FaultyDB(t, harness.Fault{Op: "queryrow", Match: "FROM diagnosis_sync_ops", Err: boom}), &engine{confidence: 0.9})
	e.a.Do("POST", "/v1/scans/sync", transport.SyncRequest{Operations: []transport.SyncOperation{{IdempotencyKey: "k", Kind: "annotate_scan", ScanID: missing}}},
		e.a.Token(owner, authz.Farmer)).Expect(t, 500)
}
