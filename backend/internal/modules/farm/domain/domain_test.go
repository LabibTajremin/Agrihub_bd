package domain

import (
	"errors"
	"math"
	"testing"
	"testing/quick"
)

func TestCoordinate(t *testing.T) {
	c, err := NewCoordinate(23.8103, 90.4125)
	if err != nil || c.LatE7 != 238103000 || c.Lat() != 23.8103 || c.Lng() != 90.4125 {
		t.Fatal(c, err)
	}
	for _, bad := range [][2]float64{{91, 0}, {-91, 0}, {0, 181}, {0, -181}, {math.NaN(), 0}, {0, math.NaN()}} {
		if _, err := NewCoordinate(bad[0], bad[1]); !errors.Is(err, ErrInvalidLocation) {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestArea_KnownConversions(t *testing.T) {
	bigha, _ := AreaFromMilli(1000, Bigha)
	if bigha.Milli(Decimal) != 33000 {
		t.Fatal("1 bigha = 33 decimals", bigha.Milli(Decimal))
	}
	ha, _ := AreaFromMilli(1000, Hectare)
	if ha.Milli(Decimal) != 247105 || ha.Milli(Acre) != 2471 || ha.Hectares() != 1 {
		t.Fatal(ha.Milli(Decimal), ha.Milli(Acre))
	}
	acre, _ := AreaFromMilli(1000, Acre)
	if acre.Milli(Decimal) != 100000 || AreaFromNano(acre.Nano()) != acre {
		t.Fatal()
	}
	if AreaFromNano(-1).Nano() != 0 || acre.Milli("rood") != 0 || len(Units()) != 4 || len(Textures()) != 6 {
		t.Fatal()
	}
}

func TestArea_Rejects(t *testing.T) {
	for _, c := range []struct {
		milli int64
		unit  Unit
	}{{0, Decimal}, {-5, Acre}, {1, "rood"}, {int64(mulDivRound(MaxAreaNano, 1000, nanoPer[Hectare])) + 1, Hectare}, {math.MaxInt64, Decimal}} {
		if _, err := AreaFromMilli(c.milli, c.unit); !errors.Is(err, ErrInvalidArea) {
			t.Errorf("%+v accepted", c)
		}
	}
}

// TestArea_RoundTripProperty: converting in and out of any unit is exact for
// every representable input.
func TestArea_RoundTripProperty(t *testing.T) {
	for _, u := range Units() {
		limit := mulDivRound(MaxAreaNano, 1000, nanoPer[u])
		prop := func(x uint64) bool {
			milli := int64(x%limit) + 1
			a, err := AreaFromMilli(milli, u)
			return err == nil && a.Milli(u) == milli && a.Nano() > 0 && uint64(a.Nano()) <= MaxAreaNano+nanoPer[u]
		}
		if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
}

func TestCatalogue(t *testing.T) {
	cs := Catalogue()
	if len(cs) != 12 {
		t.Fatal(len(cs))
	}
	for i, c := range cs {
		if i > 0 && cs[i-1].Code >= c.Code {
			t.Fatal("sorted by code")
		}
		if c.YieldKgHa.Low > c.YieldKgHa.Likely || c.YieldKgHa.Likely > c.YieldKgHa.High || c.PHMinX10 >= c.PHMaxX10 || len(c.Seasons) == 0 || len(c.CostPoishaPerHa) != 5 {
			t.Errorf("inconsistent %s", c.Code)
		}
		for _, bp := range []int{c.SeedAvailBP, c.MarketDemandBP, c.PestRiskBP} {
			if bp < 0 || bp > 10000 {
				t.Errorf("%s basis points out of range", c.Code)
			}
		}
	}
	if c, err := FindCrop("lentil"); err != nil || c.NitrogenDeltaKgHa <= 0 {
		t.Fatal("legumes fix nitrogen")
	}
	if _, err := FindCrop("kale"); !errors.Is(err, ErrCropNotFound) {
		t.Fatal()
	}
	if NextSeason(SeasonAman) != SeasonBoro || NextSeason(SeasonAus) != SeasonAman || NextSeason("winter") != "" {
		t.Fatal()
	}
	if len(Errors()) != 5 {
		t.Fatal()
	}
}
