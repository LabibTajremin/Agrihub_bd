// Package idgen issues time-ordered UUIDv7 identifiers behind an interface so
// tests can inject a deterministic sequence.
package idgen

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// Generator produces new unique identifiers.
type Generator interface {
	New() string
}

// UUIDv7 generates RFC 9562 version-7 UUIDs (time-ordered, index friendly).
type UUIDv7 struct{}

// New returns a fresh UUIDv7 string.
func (UUIDv7) New() string { return uuid.Must(uuid.NewV7()).String() }

// Sequence yields predictable, valid v7-shaped UUIDs: ...-7000-8000-<counter>.
type Sequence struct {
	mu sync.Mutex
	n  uint64
}

// New returns the next identifier in the sequence.
func (s *Sequence) New() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return fmt.Sprintf("00000000-0000-7000-8000-%012x", s.n)
}

// Valid reports whether s is a canonical UUID string.
func Valid(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
