package domain

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestCell(t *testing.T) {
	c, err := CellOf(24.849, 89.371)
	if err != nil || c.Key() != "24.8:89.4" || c.Lat() != 24.8 || c.Lng() != 89.4 {
		t.Fatal(c, err)
	}
	for _, bad := range [][2]float64{{91, 0}, {0, -181}, {math.NaN(), 0}} {
		if _, err := CellOf(bad[0], bad[1]); !errors.Is(err, ErrInvalidLocation) {
			t.Error(bad)
		}
	}
	if len(Errors()) != 2 {
		t.Fatal()
	}
}

func TestReport_StalenessIsFlagged(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	o := Observation{FetchedAt: t0}
	if r := NewReport(o, t0.Add(3*time.Hour), 3*time.Hour); r.Stale || r.AgeSeconds != 10800 {
		t.Fatal(r)
	}
	if r := NewReport(o, t0.Add(3*time.Hour+time.Second), 3*time.Hour); !r.Stale {
		t.Fatal("older than stale_after")
	}
	if r := NewReport(o, t0.Add(-time.Minute), time.Hour); r.AgeSeconds != 0 {
		t.Fatal("clock skew never yields negative age")
	}
}

func TestClimatology(t *testing.T) {
	base := Climatology(23.8, 90.4) // Dhaka
	if base["aman"] != 1600 {
		t.Fatal(base)
	}
	if Climatology(25, 88.9)["aman"] >= 1600 || Climatology(24.9, 91.9)["aman"] <= 1600 || Climatology(22.3, 90.3)["boro"] <= 150 {
		t.Fatal("regional adjustment")
	}
}
