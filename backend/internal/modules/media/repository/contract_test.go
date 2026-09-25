package repository_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
)

// runContract is the shared storage contract: every backend must pass it.
func runContract(t *testing.T, s domain.Storage, upload func(t *testing.T, signedURL string, body []byte)) {
	t.Helper()
	ctx := context.Background()
	body := []byte("leaf-bytes-0123456789")
	key := "m/owner/obj-1"

	if _, err := s.Stat(ctx, key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stat missing: %v", err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := s.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	info, err := s.Stat(ctx, key)
	if err != nil || info.Size != int64(len(body)) {
		t.Fatal(info, err)
	}
	rc, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatal("round trip")
	}
	put, err := s.PresignPut(ctx, "m/owner/obj-2", "image/jpeg", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := url.Parse(put); err != nil || u.RawQuery == "" {
		t.Fatalf("presigned put url %q", put)
	}
	upload(t, put, body)
	if info, err := s.Stat(ctx, "m/owner/obj-2"); err != nil || info.Size != int64(len(body)) {
		t.Fatal("upload through presigned url", info, err)
	}
	get, err := s.PresignGet(ctx, key, time.Minute)
	if err != nil || get == "" {
		t.Fatal(get, err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, key); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("deleted")
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal("deleting twice is fine", err)
	}
}

func httpPut(t *testing.T, client *http.Client, u string, body []byte) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, u, bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/jpeg")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("presigned upload: %d", res.StatusCode)
	}
}
