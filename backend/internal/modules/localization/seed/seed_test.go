package seed

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"testing/fstest"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
var placeholder = regexp.MustCompile(`\{(\w+)\}`)

func TestSeed_SevenLanguagesSameKeysValidFormat(t *testing.T) {
	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 7 {
		t.Fatalf("want 7 languages, got %d", len(all))
	}
	en := all["en"]
	for lang, dict := range all {
		if len(dict) != len(en) {
			t.Errorf("%s has %d keys, en has %d", lang, len(dict), len(en))
		}
		for k, v := range dict {
			if !keyPattern.MatchString(k) || v == "" {
				t.Errorf("%s: bad entry %q=%q", lang, k, v)
			}
			if ev, ok := en[k]; !ok {
				t.Errorf("%s: key %s missing in en", lang, k)
			} else if len(placeholder.FindAllString(ev, -1)) != len(placeholder.FindAllString(v, -1)) {
				t.Errorf("%s: placeholders differ for %s", lang, k)
			}
		}
	}
	if len(Keys(en)) != len(en) {
		t.Fatal()
	}
}

// TestSeed_MobileCopiesAreIdentical keeps mobile/assets/i18n in sync (make i18n-sync).
func TestSeed_MobileCopiesAreIdentical(t *testing.T) {
	names, _ := filepath.Glob("*.json")
	for _, n := range names {
		a, _ := os.ReadFile(n)
		b, err := os.ReadFile(filepath.Join("../../../../../mobile/assets/i18n", n))
		if err != nil || !bytes.Equal(a, b) {
			t.Errorf("mobile/assets/i18n/%s differs from the backend seed; run make i18n-sync", n)
		}
	}
}

func TestLoad_Errors(t *testing.T) {
	if _, err := load(fstest.MapFS{"x.json": {Data: []byte("{")}}); err == nil {
		t.Fatal("bad json")
	}
	if _, err := load(fstest.MapFS{"x.json": {Mode: 0o200 | 1<<31}}); err == nil {
		t.Fatal("unreadable")
	}
	if _, err := load(fstest.MapFS{"[.json": {Data: []byte("{}")}}); err != nil {
		t.Fatal(err)
	}
}
