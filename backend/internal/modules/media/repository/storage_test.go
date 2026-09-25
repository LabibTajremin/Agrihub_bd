package repository_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

var t0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func local(t *testing.T) repository.LocalDisk {
	return repository.LocalDisk{Root: t.TempDir(), BaseURL: "https://api.example/", Key: []byte("0123456789abcdef0123456789abcdef"), Clock: clock.NewFake(t0)}
}

func TestLocalDisk_Contract(t *testing.T) {
	l := local(t)
	runContract(t, l, func(t *testing.T, signed string, body []byte) {
		u, _ := url.Parse(signed)
		key := strings.TrimPrefix(u.Path, "/v1/media/blob/")
		if !strings.HasPrefix(signed, "https://api.example/v1/media/blob/") {
			t.Fatal(signed)
		}
		if err := l.Verify("PUT", key, u.Query().Get("exp"), u.Query().Get("sig")); err != nil {
			t.Fatal(err)
		}
		if err := l.Put(context.Background(), key, bytes.NewReader(body), int64(len(body)), "image/jpeg"); err != nil {
			t.Fatal(err)
		}
	})
}

func s3Storage(t *testing.T) repository.S3 {
	t.Helper()
	backend := s3mem.New()
	faker := gofakes3.New(backend)
	srv := httptest.NewServer(faker.Server())
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4("key", "secret", ""), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MakeBucket(context.Background(), "media", minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
		t.Fatal(err)
	}
	return repository.S3{Client: client, Bucket: "media"}
}

func TestS3_Contract(t *testing.T) {
	s := s3Storage(t)
	runContract(t, s, func(t *testing.T, signed string, body []byte) {
		httpPut(t, http.DefaultClient, signed, body)
	})
}

func TestS3_Errors(t *testing.T) {
	s := s3Storage(t)
	ctx := context.Background()
	missing := repository.S3{Client: s.Client, Bucket: "no-such-bucket"}
	if err := missing.Put(ctx, "k", strings.NewReader("x"), 1, "image/png"); err == nil {
		t.Fatal("put into a missing bucket")
	}
	if _, err := missing.PresignPut(ctx, "k", "image/png", -time.Second); err == nil {
		t.Fatal("invalid expiry")
	}
	if _, err := missing.PresignGet(ctx, "k", -time.Second); err == nil {
		t.Fatal("invalid expiry")
	}
	broken, _ := minio.New("127.0.0.1:1", &minio.Options{Creds: credentials.NewStaticV4("k", "s", ""), Region: "us-east-1", MaxRetries: 1})
	if _, err := (repository.S3{Client: broken, Bucket: "b"}).Stat(ctx, "k"); err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatal("transport errors are not 'not found'", err)
	}
}

func TestLocalDisk_SignatureAndPaths(t *testing.T) {
	l := local(t)
	sig := l.Sign("PUT", "m/a/b", t0.Add(time.Minute).Unix())
	exp := "1772323260" // t0 + 1 minute
	if err := l.Verify("PUT", "m/a/b", exp, sig); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string][3]string{
		"method":  {"GET", exp, sig},
		"expired": {"PUT", "1000", l.Sign("PUT", "m/a/b", 1000)},
		"bad exp": {"PUT", "soon", sig},
		"bad sig": {"PUT", exp, strings.Repeat("0", 64)},
	} {
		if err := l.Verify(c[0], "m/a/b", c[1], c[2]); !errors.Is(err, domain.ErrTicketInvalid) {
			t.Errorf("%s accepted", name)
		}
	}
	ctx := context.Background()
	for _, key := range []string{"", "/", "../etc/passwd", "m/../../x"} {
		if err := l.Put(ctx, key, strings.NewReader("x"), 1, ""); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("put %q: %v", key, err)
		}
		if _, err := l.Get(ctx, key); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("get %q", key)
		}
		if _, err := l.Stat(ctx, key); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("stat %q", key)
		}
		if err := l.Delete(ctx, key); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("delete %q", key)
		}
	}
}

func TestLocalDisk_PutFailures(t *testing.T) {
	ctx := context.Background()
	l := local(t)
	if err := l.Put(ctx, "m/a/short", strings.NewReader("abc"), 5, ""); !errors.Is(err, domain.ErrChecksumMismatch) {
		t.Fatal("short body", err)
	}
	if err := l.Put(ctx, "m/a/long", strings.NewReader("abcdef"), 5, ""); !errors.Is(err, domain.ErrChecksumMismatch) {
		t.Fatal("long body", err)
	}
	if err := l.Put(ctx, "m/a/err", iotest.ErrReader(errors.New("net")), 5, ""); err == nil {
		t.Fatal("reader error")
	}
	// a file where a directory is needed
	if err := os.WriteFile(filepath.Join(l.Root, "blocker"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Put(ctx, "blocker/x", strings.NewReader("x"), 1, ""); err == nil {
		t.Fatal("mkdir over a file")
	}
	if _, err := l.Stat(ctx, "blocker/x"); err == nil {
		t.Fatal("stat under a file")
	}
	if err := l.Delete(ctx, "blocker/x"); err == nil {
		t.Fatal("delete under a file")
	}
	ro := filepath.Join(l.Root, "ro")
	if err := os.MkdirAll(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() != 0 { // root ignores directory permissions
		if err := l.Put(ctx, "ro/x", strings.NewReader("x"), 1, ""); err == nil {
			t.Fatal("unwritable dir")
		}
	}
}
