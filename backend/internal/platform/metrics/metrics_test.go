package metrics

import (
	"testing"
	"time"
)

func TestRegistry_AggregatesAndSorts(t *testing.T) {
	r := NewRegistry()
	r.ObserveRequest("b", 200, time.Millisecond)
	r.ObserveRequest("a", 500, 3*time.Millisecond)
	r.ObserveRequest("a", 200, 2*time.Millisecond)
	r.ObserveRequest("a", 200, 4*time.Millisecond)
	s := r.Snapshot()
	if len(s) != 3 || s[0].Route != "a" || s[0].Status != 200 || s[0].Count != 2 || s[0].TotalMs != 6 || s[0].MaxMs != 4 {
		t.Fatalf("%+v", s)
	}
	if s[1].Status != 500 || s[2].Route != "b" {
		t.Fatalf("%+v", s)
	}
	Nop{}.ObserveRequest("x", 1, 0)
}
