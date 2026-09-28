package idgen

import (
	"testing"

	"github.com/google/uuid"
)

func TestUUIDv7_IsVersion7AndOrdered(t *testing.T) {
	g := UUIDv7{}
	a, b := g.New(), g.New()
	u, err := uuid.Parse(a)
	if err != nil || u.Version() != 7 {
		t.Fatalf("not v7: %s %v", a, err)
	}
	if a >= b {
		t.Fatalf("not time ordered: %s >= %s", a, b)
	}
}

func TestSequence_IsDeterministic(t *testing.T) {
	var s Sequence
	if got := s.New(); got != "00000000-0000-7000-8000-000000000001" {
		t.Fatal(got)
	}
	if !Valid(s.New()) {
		t.Fatal("sequence id must be valid")
	}
}

func TestValid(t *testing.T) {
	for in, want := range map[string]bool{
		"00000000-0000-7000-8000-000000000001":  true,
		"nope":                                  false,
		"urn:uuid:00000000-0000-7000-8000-0000": false,
		"zzzzzzzz-0000-7000-8000-000000000001":  false,
	} {
		if Valid(in) != want {
			t.Errorf("%s: want %v", in, want)
		}
	}
}
