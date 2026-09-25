//go:build integration

package media_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var (
	ctx  = context.Background()
	boom = errors.New("boom")
)

const (
	owner = "00000000-0000-7000-8000-0000000000a1"
	other = "00000000-0000-7000-8000-0000000000b2"
)

var policy = usecase.Policy{AllowedTypes: []string{"image/png", "image/jpeg", "audio/mpeg"}, MaxBytes: 1 << 20, PresignTTL: 15 * time.Minute}

// faulty wraps a storage and fails selected operations.
type faulty struct {
	domain.Storage
	presign, get, del bool
}

func (f faulty) PresignPut(ctx context.Context, k, ct string, ttl time.Duration) (string, error) {
	if f.presign {
		return "", boom
	}
	return f.Storage.PresignPut(ctx, k, ct, ttl)
}

func (f faulty) PresignGet(ctx context.Context, k string, ttl time.Duration) (string, error) {
	if f.presign {
		return "", boom
	}
	return f.Storage.PresignGet(ctx, k, ttl)
}

func (f faulty) Get(ctx context.Context, k string) (io.ReadCloser, error) {
	if f.get {
		return nil, boom
	}
	return f.Storage.Get(ctx, k)
}

func (f faulty) Delete(ctx context.Context, k string) error {
	if f.del {
		return boom
	}
	return f.Storage.Delete(ctx, k)
}

type env struct {
	a     *apitest.API
	m     *media.Module
	local *repository.LocalDisk
}

func setup(t *testing.T, db *database.DB, wrap func(domain.Storage) domain.Storage) env {
	t.Helper()
	a := apitest.New(t, db)
	store, local, err := media.NewStorage(config.Storage{Backend: "local", LocalDir: t.TempDir(), LocalSigningKey: "0123456789abcdef0123456789abcdef"}, "http://api.test", a.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if wrap != nil {
		store = wrap(store)
	}
	m, err := media.New(media.Deps{Kernel: a.Kernel, Storage: store, Local: local, Policy: policy})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return env{a: a, m: m, local: local}
}

func leafPNG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{uint8(255 - x*3), uint8(100 + y), 40, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func ticket(t *testing.T, e env, token, ct string, body []byte) transport.TicketResponse {
	t.Helper()
	var tr transport.TicketResponse
	e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: ct, SizeBytes: int64(len(body)), Checksum: sum(body)}, token).
		Expect(t, 201).Decode(t, &tr)
	return tr
}

func upload(t *testing.T, e env, tr transport.TicketResponse, body []byte, ct string) int {
	t.Helper()
	u, _ := url.Parse(tr.UploadURL)
	return e.a.Do("PUT", u.RequestURI(), string(body), "", "Content-Type", ct).Code
}

func TestUpload_FullFlowWithPerceptualHash(t *testing.T) {
	e := setup(t, harness.DB(t), nil)
	tok := e.a.Token(owner, authz.Guest)
	body := leafPNG(t)
	tr := ticket(t, e, tok, "image/png", body)
	if tr.Method != "PUT" || tr.Headers["Content-Type"] != "image/png" || !tr.ExpiresAt.Equal(apitest.Epoch.Add(15*time.Minute)) {
		t.Fatalf("%+v", tr)
	}
	e.a.Do("GET", "/v1/media/"+tr.MediaID, nil, tok).Expect(t, 409)
	if c := e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok).Expect(t, 409).ErrorCode(); c != "media.not_uploaded" {
		t.Fatal(c)
	}
	if code := upload(t, e, tr, body, "image/png"); code != 204 {
		t.Fatal(code)
	}
	var o transport.Object
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok).Expect(t, 200).Decode(t, &o)
	if o.Status != "uploaded" || o.PHash == "0000000000000000" || o.UploadedAt == nil {
		t.Fatalf("%+v", o)
	}
	var again transport.Object
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok).Expect(t, 200).Decode(t, &again)
	if again.PHash != o.PHash {
		t.Fatal("complete is idempotent")
	}
	var got transport.Object
	e.a.Do("GET", "/v1/media/"+tr.MediaID, nil, tok).Expect(t, 200).Decode(t, &got)
	du, _ := url.Parse(got.DownloadURL)
	res := e.a.Do("GET", du.RequestURI(), nil, "").Expect(t, 200)
	if !bytes.Equal(res.Body, body) {
		t.Fatal("download")
	}
	// reviewers may read any media; other farmers may not
	e.a.Do("GET", "/v1/media/"+tr.MediaID, nil, e.a.Token(other, authz.FieldOfficer)).Expect(t, 200)
	e.a.Do("GET", "/v1/media/"+tr.MediaID, nil, e.a.Token(other, authz.Farmer)).Expect(t, 403)
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, e.a.Token(other, authz.Farmer)).Expect(t, 403)
	v, err := e.m.Object(ctx, tr.MediaID)
	if err != nil || !v.Uploaded || v.OwnerID != owner || v.PHash == 0 {
		t.Fatal(v, err)
	}
	rc, err := e.m.Open(ctx, tr.MediaID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(b, body) {
		t.Fatal("open")
	}
	if flushed, _ := e.a.Relay.Flush(ctx); flushed.Dispatched != 1 {
		t.Fatal(flushed)
	}
}

func TestUpload_Rejections(t *testing.T) {
	e := setup(t, harness.DB(t), nil)
	tok := e.a.Token(owner, authz.Farmer)
	if c := e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: "application/pdf", SizeBytes: 1, Checksum: sum(nil)}, tok).Expect(t, 400).ErrorCode(); c != "media.unsupported_type" {
		t.Fatal(c)
	}
	if c := e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: "image/png", SizeBytes: 2 << 20, Checksum: sum(nil)}, tok).Expect(t, 400).ErrorCode(); c != "media.too_large" {
		t.Fatal(c)
	}
	// checksum mismatch: declared digest differs from the bytes sent
	body := leafPNG(t)
	var tr transport.TicketResponse
	e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: "image/png", SizeBytes: int64(len(body)), Checksum: sum([]byte("other"))}, tok).Expect(t, 201).Decode(t, &tr)
	upload(t, e, tr, body, "image/png")
	if c := e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok).Expect(t, 400).ErrorCode(); c != "media.checksum_mismatch" {
		t.Fatal(c)
	}
	if c := e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok).Expect(t, 409).ErrorCode(); c != "media.not_uploaded" {
		t.Fatal("mismatching blob is deleted", c)
	}
	// valid checksum but not a decodable image
	junk := []byte("definitely not a png")
	tj := ticket(t, e, tok, "image/png", junk)
	upload(t, e, tj, junk, "image/png")
	if c := e.a.Do("POST", "/v1/media/"+tj.MediaID+"/complete", nil, tok).Expect(t, 400).ErrorCode(); c != "media.unsupported_type" {
		t.Fatal(c)
	}
	// audio has no perceptual hash
	clip := []byte("ID3-audio")
	ta := ticket(t, e, tok, "audio/mpeg", clip)
	upload(t, e, ta, clip, "audio/mpeg")
	var o transport.Object
	e.a.Do("POST", "/v1/media/"+ta.MediaID+"/complete", nil, tok).Expect(t, 200).Decode(t, &o)
	if o.PHash != "0000000000000000" {
		t.Fatal(o.PHash)
	}
	// blob endpoint: wrong content type, bad signature, tampered key
	u, _ := url.Parse(ta.UploadURL)
	if code := e.a.Do("PUT", u.RequestURI(), "x", "", "Content-Type", "image/png").Code; code != 401 {
		t.Fatal("content type must match the ticket", code)
	}
	e.a.Do("PUT", u.Path+"?exp=1&sig=00", "x", "").Expect(t, 401)
	e.a.Do("GET", u.Path+"?exp=1&sig=00", nil, "").Expect(t, 401)
	signedOther := e.local.Sign("PUT", "m/x/00000000-0000-7000-8000-000000000999", 9999999999)
	e.a.Do("PUT", "/v1/media/blob/m/x/00000000-0000-7000-8000-000000000999?exp=9999999999&sig="+signedOther, "x", "").Expect(t, 404)
	e.a.Do("GET", "/v1/media/00000000-0000-7000-8000-000000000999", nil, tok).Expect(t, 404)
	e.a.Do("GET", "/v1/media/nope", nil, tok).Expect(t, 400)
	e.a.Do("POST", "/v1/media/nope/complete", nil, tok).Expect(t, 400)
	e.a.Do("POST", "/v1/media/tickets", "{", tok).Expect(t, 400)
	e.a.Do("POST", "/v1/media/00000000-0000-7000-8000-000000000999/complete", nil, tok).Expect(t, 404)
	if _, err := e.m.Object(ctx, "00000000-0000-7000-8000-000000000999"); err == nil {
		t.Fatal()
	}
	if _, err := e.m.Open(ctx, "00000000-0000-7000-8000-000000000999"); err == nil {
		t.Fatal()
	}
}

func TestFailures_StorageAndDatabase(t *testing.T) {
	body := leafPNG(t)
	tok := func(e env) string { return e.a.Token(owner, authz.Farmer) }

	e := setup(t, harness.DB(t), func(s domain.Storage) domain.Storage { return faulty{Storage: s, presign: true} })
	e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: "image/png", SizeBytes: 3, Checksum: sum(nil)}, tok(e)).Expect(t, 500)

	e = setup(t, harness.FaultyDB(t, harness.Fault{Op: "exec", Match: "INSERT INTO media_objects", Err: boom}), nil)
	e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: "image/png", SizeBytes: 3, Checksum: sum(nil)}, tok(e)).Expect(t, 500)

	for name, fault := range map[string]harness.Fault{
		"mark":    {Op: "exec", Match: "UPDATE media_objects", Err: boom},
		"publish": {Op: "exec", Match: "INSERT INTO outbox", Err: boom},
	} {
		e = setup(t, harness.FaultyDB(t, fault), nil)
		tr := ticket(t, e, tok(e), "image/png", body)
		upload(t, e, tr, body, "image/png")
		if code := e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok(e)).Code; code != 500 {
			t.Errorf("%s: %d", name, code)
		}
	}

	var getFails, delFails bool
	e = setup(t, harness.DB(t), func(s domain.Storage) domain.Storage {
		return &switchable{Storage: s, get: &getFails, del: &delFails}
	})
	tr := ticket(t, e, tok(e), "image/png", body)
	upload(t, e, tr, body, "image/png")
	getFails = true
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok(e)).Expect(t, 500)
	getFails = false
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok(e)).Expect(t, 200)
	getFails = true
	u, _ := url.Parse(func() string {
		var o transport.Object
		getFails = false
		e.a.Do("GET", "/v1/media/"+tr.MediaID, nil, tok(e)).Decode(t, &o)
		return o.DownloadURL
	}())
	getFails = true
	e.a.Do("GET", u.RequestURI(), nil, "").Expect(t, 500)
	getFails = false

	// checksum mismatch whose cleanup also fails still reports the mismatch
	delFails = true
	var bad transport.TicketResponse
	e.a.Do("POST", "/v1/media/tickets", transport.TicketRequest{ContentType: "image/png", SizeBytes: int64(len(body)), Checksum: sum([]byte("x"))}, tok(e)).Decode(t, &bad)
	upload(t, e, bad, body, "image/png")
	e.a.Do("POST", "/v1/media/"+bad.MediaID+"/complete", nil, tok(e)).Expect(t, 400)

	e = setup(t, harness.DB(t), func(s domain.Storage) domain.Storage { return faulty{Storage: s} })
	tr = ticket(t, e, tok(e), "image/png", body)
	upload(t, e, tr, body, "image/png")
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, tok(e)).Expect(t, 200)
}

type switchable struct {
	domain.Storage
	get, del *bool
}

func (s *switchable) Get(ctx context.Context, k string) (io.ReadCloser, error) {
	if *s.get {
		return nil, boom
	}
	return s.Storage.Get(ctx, k)
}

func (s *switchable) Delete(ctx context.Context, k string) error {
	if *s.del {
		return boom
	}
	return s.Storage.Delete(ctx, k)
}

func TestUseCases_RequireAuthAndPresignGetFailure(t *testing.T) {
	db := harness.DB(t)
	e := setup(t, db, nil)
	body := leafPNG(t)
	tr := ticket(t, e, e.a.Token(owner, authz.Farmer), "image/png", body)
	upload(t, e, tr, body, "image/png")
	e.a.Do("POST", "/v1/media/"+tr.MediaID+"/complete", nil, e.a.Token(owner, authz.Farmer)).Expect(t, 200)

	d := usecase.Deps{Objects: repository.Objects{DB: db}, Storage: faulty{Storage: e.local, presign: true}, Tx: db,
		Clock: e.a.Clock, IDs: e.a.Kernel.IDs, Events: e.a.Kernel.Events, Factory: e.a.Kernel.Factory(), Policy: policy}
	farmer := authn.WithPrincipal(ctx, authn.Principal{Subject: owner, Role: authz.Farmer})
	if _, err := (usecase.GetObject{Deps: d}).Execute(farmer, tr.MediaID); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := (usecase.CreateTicket{Deps: d}).Execute(ctx, usecase.TicketInput{}); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.Complete{Deps: d}).Execute(ctx, tr.MediaID); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.GetObject{Deps: d}).Execute(ctx, tr.MediaID); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, _, err := media.NewStorage(config.Storage{Backend: "s3", S3Endpoint: "host/with/path"}, "", e.a.Clock); err == nil {
		t.Fatal("invalid endpoint")
	}
	s3, local, err := media.NewStorage(config.Storage{Backend: "s3", S3Endpoint: "localhost:9000", S3Bucket: "b"}, "", e.a.Clock)
	if err != nil || local != nil || s3 == nil {
		t.Fatal(err)
	}
	if len(media.Errors()) != 6 {
		t.Fatal()
	}
}
