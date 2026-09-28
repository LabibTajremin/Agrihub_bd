// Package metrics records request metrics behind an interface. The in-process
// Registry has no global state; it is created by the composition root.
package metrics

import (
	"sort"
	"strconv"
	"sync"
	"time"
)

// Recorder observes completed HTTP requests.
type Recorder interface {
	ObserveRequest(route string, status int, d time.Duration)
}

// Nop discards observations (Null Object).
type Nop struct{}

// ObserveRequest implements Recorder.
func (Nop) ObserveRequest(string, int, time.Duration) {}

// Series is one aggregated (route, status) counter.
type Series struct {
	Route   string  `json:"route"`
	Status  int     `json:"status"`
	Count   int64   `json:"count"`
	TotalMs float64 `json:"total_ms"`
	MaxMs   float64 `json:"max_ms"`
}

// Registry aggregates observations in memory.
type Registry struct {
	mu     sync.Mutex
	series map[string]*Series
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{series: map[string]*Series{}} }

// ObserveRequest implements Recorder.
func (r *Registry) ObserveRequest(route string, status int, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := route + "|" + strconv.Itoa(status)
	s, ok := r.series[key]
	if !ok {
		s = &Series{Route: route, Status: status}
		r.series[key] = s
	}
	ms := float64(d) / float64(time.Millisecond)
	s.Count++
	s.TotalMs += ms
	s.MaxMs = max(s.MaxMs, ms)
}

// Snapshot returns all series sorted by route then status.
func (r *Registry) Snapshot() []Series {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Series, 0, len(r.series))
	for _, s := range r.series {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Route != out[j].Route {
			return out[i].Route < out[j].Route
		}
		return out[i].Status < out[j].Status
	})
	return out
}
