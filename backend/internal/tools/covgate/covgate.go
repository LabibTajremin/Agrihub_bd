// Package covgate parses Go coverage profiles, merges duplicate blocks produced by
// -coverpkg runs, applies the .coverageignore exclusions and enforces a per-package
// coverage threshold.
package covgate

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Block is one coverage block of a profile.
type Block struct {
	File     string
	Range    string
	NumStmts int
	Count    int
}

// PackageResult is the coverage of one package after merging and exclusion.
type PackageResult struct {
	Package    string
	Statements int
	Covered    int
}

// Percent returns the covered share of statements, 100 for empty packages.
func (p PackageResult) Percent() float64 {
	if p.Statements == 0 {
		return 100
	}
	return float64(p.Covered) * 100 / float64(p.Statements)
}

// ParseProfile reads a coverage profile and merges duplicate blocks by max count.
func ParseProfile(r io.Reader) ([]Block, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	merged := map[string]*Block{}
	var order []string
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "mode:") {
			continue
		}
		b, err := parseLine(text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		key := b.File + ":" + b.Range
		if existing, ok := merged[key]; ok {
			existing.Count = max(existing.Count, b.Count)
			continue
		}
		merged[key] = &b
		order = append(order, key)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]Block, 0, len(order))
	for _, k := range order {
		out = append(out, *merged[k])
	}
	return out, nil
}

func parseLine(text string) (Block, error) {
	colon := strings.LastIndex(text, ":")
	if colon < 0 {
		return Block{}, fmt.Errorf("malformed block %q", text)
	}
	fields := strings.Fields(text[colon+1:])
	if len(fields) != 3 {
		return Block{}, fmt.Errorf("malformed block %q", text)
	}
	stmts, err := strconv.Atoi(fields[1])
	if err != nil {
		return Block{}, fmt.Errorf("statements: %w", err)
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return Block{}, fmt.Errorf("count: %w", err)
	}
	return Block{File: text[:colon], Range: fields[0], NumStmts: stmts, Count: count}, nil
}

// Matcher decides whether a module-relative file path is excluded.
type Matcher struct {
	patterns []*regexp.Regexp
	raw      []string
}

// ParseIgnore reads .coverageignore content: one glob per line, '#' comments allowed.
func ParseIgnore(r io.Reader) (*Matcher, error) {
	sc := bufio.NewScanner(r)
	m := &Matcher{}
	for sc.Scan() {
		p := strings.TrimSpace(sc.Text())
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		m.raw = append(m.raw, p)
		m.patterns = append(m.patterns, globToRegexp(p))
	}
	return m, sc.Err()
}

// Patterns returns the parsed glob patterns in file order.
func (m *Matcher) Patterns() []string { return append([]string(nil), m.raw...) }

// Match reports whether rel is excluded.
func (m *Matcher) Match(rel string) bool {
	for _, re := range m.patterns {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

func globToRegexp(glob string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch {
		case c == '*' && i+1 < len(glob) && glob[i+1] == '*':
			sb.WriteString(".*")
			i++
			if i+1 < len(glob) && glob[i+1] == '/' {
				i++
				sb.WriteString("/?")
			}
		case c == '*':
			sb.WriteString("[^/]*")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	return regexp.MustCompile(sb.String())
}

// Summarize groups blocks by package, dropping excluded files. modulePath is
// stripped from file paths before matching.
func Summarize(blocks []Block, modulePath string, ignore *Matcher) []PackageResult {
	byPkg := map[string]*PackageResult{}
	prefix := strings.TrimSuffix(modulePath, "/") + "/"
	for _, b := range blocks {
		rel := strings.TrimPrefix(b.File, prefix)
		if ignore.Match(rel) {
			continue
		}
		pkg := path.Dir(rel)
		res, ok := byPkg[pkg]
		if !ok {
			res = &PackageResult{Package: pkg}
			byPkg[pkg] = res
		}
		res.Statements += b.NumStmts
		if b.Count > 0 {
			res.Covered += b.NumStmts
		}
	}
	out := make([]PackageResult, 0, len(byPkg))
	for _, r := range byPkg {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Package < out[j].Package })
	return out
}

// Check writes a report and returns the packages under threshold.
func Check(w io.Writer, results []PackageResult, threshold float64) []PackageResult {
	var failed []PackageResult
	total, covered := 0, 0
	for _, r := range results {
		total += r.Statements
		covered += r.Covered
		if r.Percent() < threshold {
			failed = append(failed, r)
			fmt.Fprintf(w, "FAIL %6.2f%% %s (%d/%d)\n", r.Percent(), r.Package, r.Covered, r.Statements)
		}
	}
	overall := PackageResult{Statements: total, Covered: covered}
	fmt.Fprintf(w, "coverage: %.2f%% of %d statements across %d packages (gate %.1f%%)\n",
		overall.Percent(), total, len(results), threshold)
	return failed
}

// UncoveredBlocks lists uncovered, non-excluded blocks for diagnostics.
func UncoveredBlocks(blocks []Block, modulePath string, ignore *Matcher) []string {
	prefix := strings.TrimSuffix(modulePath, "/") + "/"
	var out []string
	for _, b := range blocks {
		rel := strings.TrimPrefix(b.File, prefix)
		if b.Count == 0 && !ignore.Match(rel) {
			out = append(out, rel+":"+b.Range)
		}
	}
	return out
}
