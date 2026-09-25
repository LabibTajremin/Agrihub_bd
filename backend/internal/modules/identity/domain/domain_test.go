package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	ok := map[string]string{
		"01711-000000":        "+8801711000000",
		"8801711000000":       "+8801711000000",
		"+8801711000000":      "+8801711000000",
		" +44 (20) 7946 0958": "+442079460958",
	}
	for in, want := range ok {
		got, err := NormalizePhone(in)
		if err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "012", "01211000000", "+0123", "abc", "8801211000000"} {
		if _, err := NormalizePhone(bad); !errors.Is(err, ErrInvalidPhone) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSessionActive(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := Session{ExpiresAt: now.Add(time.Hour)}
	if !s.Active(now) || s.Active(now.Add(time.Hour)) {
		t.Fatal()
	}
	s.RevokedAt = &now
	if s.Active(now) {
		t.Fatal()
	}
}

func TestLanguagesAndCatalogue(t *testing.T) {
	if !ValidLanguage("ar") || ValidLanguage("xx") || len(Languages) != 7 {
		t.Fatal()
	}
	if len(Errors()) != 12 {
		t.Fatal(len(Errors()))
	}
}
