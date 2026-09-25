// Package domain holds the localization model: languages, dictionary entries,
// voice assets and the lock-free in-memory catalog of snapshots.
package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Language is a supported UI language. IsRTL drives layout direction on clients.
type Language struct {
	Code       string
	Name       string
	NativeName string
	IsRTL      bool
	IsDefault  bool
	HasVoice   bool
	Version    int64
}

// VoiceAsset is a pre-recorded clip for a dictionary key (no TTS).
type VoiceAsset struct {
	Key        string
	MediaID    string
	URL        string
	DurationMs int
	Checksum   string
	Version    int64
}

// Errors.
var (
	ErrLanguageNotFound = errs.NotFound("i18n.language_not_found")
	ErrInvalidEntry     = errs.Validation("i18n.invalid_entry")
)

// Errors lists the localization error catalogue.
func Errors() []*errs.Error { return []*errs.Error{ErrLanguageNotFound, ErrInvalidEntry} }

// KeyPattern is the dictionary key format: screen.section.element.
var KeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// ValidKey reports whether key follows the convention.
func ValidKey(key string) bool { return len(key) <= 128 && KeyPattern.MatchString(key) }

// Namespace is the first key segment (the screen).
func Namespace(key string) string {
	ns, _, _ := strings.Cut(key, ".")
	return ns
}

// ETag is a strong entity tag over version and sorted content.
func ETag(version int64, entries map[string]string) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte(strconv.FormatInt(version, 10)))
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte{1})
		h.Write([]byte(entries[k]))
	}
	return `"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}

// Snapshot is an immutable full dictionary of one language.
type Snapshot struct {
	Lang    string
	Version int64
	Entries map[string]string
	ETag    string
}

// NewSnapshot builds a snapshot and its ETag.
func NewSnapshot(lang string, version int64, entries map[string]string) *Snapshot {
	return &Snapshot{Lang: lang, Version: version, Entries: entries, ETag: ETag(version, entries)}
}

// Catalog publishes snapshots through an atomic pointer to an immutable map.
// Reads are O(1), lock-free and allocation-free; an update copies the outer
// map and swaps the pointer (copy-on-write), so readers never block.
type Catalog struct {
	p atomic.Pointer[map[string]*Snapshot]
}

// Get returns the snapshot of lang.
func (c *Catalog) Get(lang string) (*Snapshot, bool) {
	m := c.p.Load()
	if m == nil {
		return nil, false
	}
	s, ok := (*m)[lang]
	return s, ok
}

// Lookup returns the value of key in lang.
func (c *Catalog) Lookup(lang, key string) (string, bool) {
	s, ok := c.Get(lang)
	if !ok {
		return "", false
	}
	v, ok := s.Entries[key]
	return v, ok
}

// Publish installs s, retrying on concurrent publishers (CAS loop).
func (c *Catalog) Publish(s *Snapshot) {
	for {
		old := c.p.Load()
		next := map[string]*Snapshot{}
		if old != nil {
			for k, v := range *old {
				next[k] = v
			}
		}
		next[s.Lang] = s
		if c.p.CompareAndSwap(old, &next) {
			return
		}
	}
}

// Store is the persistence port.
type Store interface {
	Languages(ctx context.Context) ([]Language, error)
	Language(ctx context.Context, code string) (Language, error)
	// Entries returns entries with version > since.
	Entries(ctx context.Context, lang string, since int64) (map[string]string, int64, error)
	// Version returns the highest entry version of lang (0 when empty).
	Version(ctx context.Context, lang string) (int64, error)
	// Upsert writes entries whose value changed, stamping a new version; it
	// returns the resulting language version and the number of changed keys.
	Upsert(ctx context.Context, lang string, entries map[string]string, at time.Time) (int64, int, error)
	VoiceAssets(ctx context.Context, lang string, since int64) ([]VoiceAsset, error)
	UpsertVoice(ctx context.Context, lang string, assets []VoiceAsset, at time.Time) (int64, error)
}
