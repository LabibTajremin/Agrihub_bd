package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
)

// LocalDisk stores blobs under a directory (development and tests). Its
// presigned URLs point back at the API (/v1/media/blob/{key}) and carry an
// HMAC signature with an expiry, mirroring S3 presigned URLs.
type LocalDisk struct {
	Root    string
	BaseURL string
	Key     []byte
	Clock   clock.Clock
}

func (l LocalDisk) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if clean == "/" || strings.Contains(key, "..") {
		return "", domain.ErrNotFound
	}
	return filepath.Join(l.Root, clean), nil
}

// Put writes the blob atomically (temp file + rename).
func (l LocalDisk) Put(_ context.Context, key string, r io.Reader, size int64, _ string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	tmp, err := createIn(filepath.Dir(p))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(r, size+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != size {
		err = domain.ErrChecksumMismatch.WithField("size", "does not match declared size")
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func createIn(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, ".upload-*")
}

// Get opens the blob.
func (l LocalDisk) Get(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // path is confined to Root by path()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, domain.ErrNotFound
	}
	return f, err
}

// Stat reports the blob size.
func (l LocalDisk) Stat(_ context.Context, key string) (domain.ObjectInfo, error) {
	p, err := l.path(key)
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.ObjectInfo{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ObjectInfo{}, err
	}
	return domain.ObjectInfo{Size: fi.Size()}, nil
}

// Delete removes the blob (absent blobs are fine).
func (l LocalDisk) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Sign returns the hex HMAC of method, key and expiry.
func (l LocalDisk) Sign(method, key string, exp int64) string {
	m := hmac.New(sha256.New, l.Key)
	m.Write([]byte(method + "\n" + key + "\n" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a signed request for key.
func (l LocalDisk) Verify(method, key, expRaw, sig string) error {
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || l.Clock.Now().Unix() > exp || !hmac.Equal([]byte(sig), []byte(l.Sign(method, key, exp))) {
		return domain.ErrTicketInvalid
	}
	return nil
}

func (l LocalDisk) presign(method, key string, ttl time.Duration) string {
	exp := l.Clock.Now().Add(ttl).Unix()
	q := url.Values{"exp": {strconv.FormatInt(exp, 10)}, "sig": {l.Sign(method, key, exp)}}
	return strings.TrimSuffix(l.BaseURL, "/") + "/v1/media/blob/" + key + "?" + q.Encode()
}

// PresignPut returns a signed upload URL (PUT).
func (l LocalDisk) PresignPut(_ context.Context, key, _ string, ttl time.Duration) (string, error) {
	return l.presign("PUT", key, ttl), nil
}

// PresignGet returns a signed download URL (GET).
func (l LocalDisk) PresignGet(_ context.Context, key string, ttl time.Duration) (string, error) {
	return l.presign("GET", key, ttl), nil
}
