// Package seed embeds the canonical dictionaries (one flat key→value JSON per
// language). mobile/assets/i18n holds byte-identical copies (make i18n-sync).
package seed

import (
	"embed"
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.json
var files embed.FS

// Load returns every seed dictionary keyed by language code.
func Load() (map[string]map[string]string, error) { return load(files) }

func load(fsys fs.FS) (map[string]map[string]string, error) {
	names, _ := fs.Glob(fsys, "*.json") // a constant pattern cannot be malformed
	out := map[string]map[string]string{}
	for _, n := range names {
		raw, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(n, ".json")] = m
	}
	return out, nil
}

// Keys returns the sorted key set of one dictionary.
func Keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
