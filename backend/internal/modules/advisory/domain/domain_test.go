package domain

import (
	"container/heap"
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"testing"
	"testing/quick"
)

var w = Weights{Soil: 0.30, Water: 0.25, Pest: 0.15, Market: 0.20, Seed: 0.10}

func crops() []Crop {
	return []Crop{
		{Code: "rice_aman", Seasons: []string{"aman"}, WaterNeedMM: 1100, Textures: []string{"clay", "clay_loam"}, PHMinX10: 50, PHMaxX10: 70,
			NitrogenDeltaKgHa: -90, YieldKgHa: Yield{3000, 4500, 5500}, PricePoishaPerKg: 3000, SeedAvailBP: 9000, MarketDemandBP: 8000, PestRiskBP: 4000,
			CostPoishaPerHa: map[string]int64{"seed": 300000, "labour": 2500000}},
		{Code: "lentil", Seasons: []string{"boro"}, WaterNeedMM: 250, Textures: []string{"loam"}, PHMinX10: 60, PHMaxX10: 75, NitrogenDeltaKgHa: 40,
			YieldKgHa: Yield{900, 1300, 1700}, PricePoishaPerKg: 10000, SeedAvailBP: 7000, MarketDemandBP: 8000, PestRiskBP: 2000},
		{Code: "wheat", Seasons: []string{"boro"}, WaterNeedMM: 350, Textures: []string{"loam", "clay_loam"}, PHMinX10: 60, PHMaxX10: 75, NitrogenDeltaKgHa: -80,
			YieldKgHa: Yield{2500, 3500, 4200}, PricePoishaPerKg: 3500, SeedAvailBP: 8000, MarketDemandBP: 7000, PestRiskBP: 3000},
		{Code: "mungbean", Seasons: []string{"aus"}, WaterNeedMM: 300, Textures: []string{"loam"}, PHMinX10: 62, PHMaxX10: 72, NitrogenDeltaKgHa: 35,
			SeedAvailBP: 7000, MarketDemandBP: 7500, PestRiskBP: 3000},
		{Code: "jute", Seasons: []string{"aus"}, WaterNeedMM: 600, Textures: []string{"clay_loam"}, PHMinX10: 60, PHMaxX10: 75, NitrogenDeltaKgHa: -40,
			SeedAvailBP: 7500, MarketDemandBP: 6000, PestRiskBP: 2500},
	}
}

func TestWeights_Validate(t *testing.T) {
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Weights{{Soil: 1.1}, {Soil: -0.1, Water: 1.1}, {Soil: 0.5}, {Soil: math.NaN()}} {
		if !errors.Is(bad.Validate(), ErrInvalidInput) {
			t.Errorf("%+v accepted", bad)
		}
	}
	if len(Errors()) != 3 {
		t.Fatal()
	}
}

// TestScore_RangeProperty: for any criteria in [0,1] and any valid weights the
// score is within [0,100] (§6.3).
func TestScore_RangeProperty(t *testing.T) {
	prop := func(a, b, c, d, e, w1, w2, w3, w4 uint16) bool {
		f := func(x uint16) float64 { return float64(x) / 65535 }
		ws := []float64{f(w1), f(w2), f(w3), f(w4), 1}
		sum := 0.0
		for _, x := range ws {
			sum += x
		}
		wt := Weights{ws[0] / sum, ws[1] / sum, ws[2] / sum, ws[3] / sum, ws[4] / sum}
		s := Score(Criteria{f(a), f(b), f(c), f(d), f(e)}, wt)
		return s >= 0 && s <= 100
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
	if Score(Criteria{1, 1, 1, 1, 1}, w) != 100 || Score(Criteria{}, w) != 0 {
		t.Fatal("bounds")
	}
}

// TestNormalise_BoundsProperty: every criterion lies in [0,1] for any input.
func TestNormalise_BoundsProperty(t *testing.T) {
	cs := crops()
	textures := []string{"", "clay", "loam", "sandy"}
	irr := []string{"none", "partial", "full", "?"}
	prop := func(ph, rain int16, ci, ti, ii uint8, pest, market, seed int16) bool {
		c := cs[int(ci)%len(cs)]
		c.PestRiskBP, c.MarketDemandBP, c.SeedAvailBP = int(pest), int(market), int(seed)
		f := Field{Texture: textures[int(ti)%4], PHx10: int(ph), Irrigation: irr[int(ii)%4]}
		cr := Normalise(f, c, int(rain))
		for _, x := range []float64{cr.Soil, cr.Water, cr.Pest, cr.Market, cr.Seed} {
			if x < 0 || x > 1 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalise_Semantics(t *testing.T) {
	rice := crops()[0]
	ideal := Normalise(Field{Texture: "clay", PHx10: 60, Irrigation: "full"}, rice, 1100)
	if ideal.Soil != 1 || ideal.Water != 1 {
		t.Fatal(ideal)
	}
	unknown := Normalise(Field{}, rice, 0)
	if math.Abs(unknown.Soil-(0.6*0.4+0.4*0.5)) > 1e-12 || unknown.Water != 0 {
		t.Fatal("unknown soil is neutral, no water is zero", unknown)
	}
	acid := Normalise(Field{Texture: "sandy", PHx10: 35}, rice, 550)
	if acid.Soil != 0 || acid.Water != 0.5 {
		t.Fatal(acid)
	}
	partial := Normalise(Field{Irrigation: "partial"}, rice, 0)
	if partial.Water != 0.5 {
		t.Fatal(partial)
	}
	lentil := crops()[1]
	flooded := Normalise(Field{}, lentil, 1000) // rain = 4× need → fully waterlogged
	if flooded.Water != 0 {
		t.Fatal(flooded)
	}
	wet := Normalise(Field{}, lentil, 750) // 3× need → half penalty
	if wet.Water != 0.5 {
		t.Fatal(wet)
	}
}

// TestTopN_MatchesFullSortProperty: heap selection equals sorting everything.
func TestTopN_MatchesFullSortProperty(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for trial := range 300 {
		n := r.IntN(40)
		all := make([]Scored, n)
		for i := range all {
			all[i] = Scored{Crop: Crop{Code: string(rune('a' + r.IntN(26)))}, Score: float64(r.IntN(5))}
		}
		k := r.IntN(8)
		got := TopN(all, k)
		ref := append([]Scored(nil), all...)
		sort.Slice(ref, func(i, j int) bool { return better(ref[i], ref[j]) })
		if k > len(ref) {
			k = len(ref)
		}
		for i := range k {
			if got[i].Score != ref[i].Score || got[i].Crop.Code != ref[i].Crop.Code {
				t.Fatalf("trial %d: %v vs %v", trial, got, ref[:k])
			}
		}
		if len(got) != k {
			t.Fatal(len(got), k)
		}
	}
	if TopN([]Scored{{}}, 0) != nil {
		t.Fatal()
	}
	h := worstFirst{}
	heap.Push(&h, Scored{Score: 2})
	heap.Push(&h, Scored{Score: 1})
	if heap.Pop(&h).(Scored).Score != 1 {
		t.Fatal("root is the weakest candidate")
	}
}

func TestRank_FiltersBySeasonAndIsDeterministic(t *testing.T) {
	f := Field{Texture: "loam", PHx10: 65, Irrigation: "partial"}
	top := Rank(f, crops(), "boro", 150, w, 5)
	if len(top) != 2 || top[0].Crop.Code != "lentil" || top[0].Score < top[1].Score {
		t.Fatalf("%+v", top)
	}
	if len(Rank(f, crops(), "winter", 0, w, 5)) != 0 {
		t.Fatal()
	}
}

// TestROI_IdentityProperty: net = gross − Σcosts for every input (§6.6).
func TestROI_IdentityProperty(t *testing.T) {
	prop := func(low, likely, high uint32, price uint32, c1, c2, c3 uint32) bool {
		costs := map[string]int64{"a": int64(c1), "b": int64(c2), "c": int64(c3)}
		y := Yield{int64(low), int64(likely), int64(high)}
		r := EstimateROI(y, int64(price), costs)
		total := int64(c1) + int64(c2) + int64(c3)
		return r.TotalCost == total && r.Net.Low == r.Gross.Low-total && r.Net.Likely == r.Gross.Likely-total &&
			r.Net.High == r.Gross.High-total && r.Gross.Likely == int64(likely)*int64(price)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
	r := EstimateROI(Yield{1, 2, 3}, 100, nil)
	if r.ReturnBP != 0 || r.Net.Likely != 200 {
		t.Fatal(r)
	}
	if EstimateROI(Yield{0, 150, 0}, 100, map[string]int64{"x": 10000}).ReturnBP != 5000 {
		t.Fatal("50% return")
	}
}

func TestAreaScaling(t *testing.T) {
	ha := int64(nanoPerHectare)
	if y := EstimateYield(Yield{3000, 4500, 5500}, ha/2); y != (Yield{1500, 2250, 2750}) {
		t.Fatal(y)
	}
	if scaleByArea(0, ha) != 0 || scaleByArea(5, 0) != 0 {
		t.Fatal()
	}
	c := DefaultCosts(map[string]int64{"seed": 300000}, 2*ha)
	if c["seed"] != 600000 {
		t.Fatal(c)
	}
	if scaleByArea(1, ha/2) != 1 || scaleByArea(1, ha/2-1) != 0 {
		t.Fatal("rounds half up")
	}
}

func TestTopoSort_CycleIsTypedError(t *testing.T) {
	g := NewGraph()
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "a")
	if _, err := g.TopoSort(); !errors.Is(err, ErrRotationCycle) {
		t.Fatal(err)
	}
	if _, err := PlanRotation(g, "a", []string{"x"}, nil, nil, 0); !errors.Is(err, ErrRotationCycle) {
		t.Fatal(err)
	}
	d := NewGraph()
	d.AddEdge("a", "b")
	d.AddEdge("a", "c")
	d.AddEdge("b", "c")
	d.AddNode("a")
	order, err := d.TopoSort()
	if err != nil || order[0] != "a" || order[2] != "c" {
		t.Fatal(order, err)
	}
}

func TestPlanRotation_NitrogenBalance(t *testing.T) {
	cs := crops()
	byCode := map[string]Crop{}
	for _, c := range cs {
		byCode[c.Code] = c
	}
	seasons := SeasonsFrom("boro", 4) // boro, aus, aman, boro
	scores := map[string]float64{"wheat": 80, "lentil": 70, "jute": 75, "mungbean": 60, "rice_aman": 90}
	g := BuildRotationGraph("rice_aman", seasons, cs)

	rich, err := PlanRotation(g, "0:rice_aman", seasons, byCode, scores, 300)
	if err != nil || rich.Deficit {
		t.Fatal(err)
	}
	got := []string{}
	for _, s := range rich.Steps {
		got = append(got, s.Crop)
	}
	// ample nitrogen: best score each season (wheat 80 → jute 75 → rice 90 → wheat 80)
	if len(got) != 4 || got[0] != "wheat" || got[1] != "jute" || got[2] != "rice_aman" || got[3] != "wheat" {
		t.Fatal(got)
	}
	if rich.Steps[0].NitrogenBefore != 300 || rich.Steps[0].NitrogenAfter != 220 {
		t.Fatal(rich.Steps[0])
	}

	poor, _ := PlanRotation(g, "0:rice_aman", seasons, byCode, scores, 50)
	// low nitrogen: the legume first, then the best balance-keeping crop, then rice forces a deficit
	if poor.Steps[0].Crop != "lentil" || poor.Steps[1].Crop != "jute" || poor.Steps[2].NitrogenAfter >= 0 || !poor.Deficit {
		t.Fatalf("%+v", poor)
	}
	tie := map[string]float64{}
	tied, _ := PlanRotation(g, "0:rice_aman", seasons, byCode, tie, 500)
	if tied.Steps[0].Crop != "lentil" {
		t.Fatal("equal scores: prefer the nitrogen-restoring crop", tied.Steps[0])
	}
	sameN := map[string]Crop{"a": {Code: "a", NitrogenDeltaKgHa: -10}, "b": {Code: "b", NitrogenDeltaKgHa: -10}}
	h := NewGraph()
	h.AddEdge("0:s", "1:b")
	h.AddEdge("0:s", "1:a")
	r, _ := PlanRotation(h, "0:s", []string{"x", "y"}, sameN, map[string]float64{}, 0)
	if r.Steps[0].Crop != "a" || len(r.Steps) != 1 {
		t.Fatal("ties fall back to code order; stops at the last layer", r)
	}
	deficits := map[string]Crop{"a": {Code: "a", NitrogenDeltaKgHa: -30}, "c": {Code: "c", NitrogenDeltaKgHa: -10}}
	k := NewGraph()
	k.AddEdge("0:s", "1:a")
	k.AddEdge("0:s", "1:c")
	if r, _ := PlanRotation(k, "0:s", []string{"x"}, deficits, map[string]float64{"a": 99}, 0); r.Steps[0].Crop != "c" {
		t.Fatal("in deficit, minimise nitrogen loss regardless of score", r)
	}
	r2, _ := PlanRotation(h, "0:s", []string{"x"}, sameN, map[string]float64{}, 100)
	if r2.Steps[0].Crop != "a" {
		t.Fatal(r2)
	}
}

func TestSeasons(t *testing.T) {
	if SeasonsFrom("winter", 2) != nil || len(SeasonsFrom("aus", 4)) != 4 || SeasonsFrom("aus", 2)[1] != "aman" {
		t.Fatal()
	}
	want := map[int]string{1: "boro", 2: "boro", 3: "aus", 6: "aus", 7: "aman", 10: "aman", 11: "boro", 12: "boro"}
	for m, s := range want {
		if SeasonAt(m) != s {
			t.Errorf("%d", m)
		}
	}
	if irrigationShare("?") != 0 {
		t.Fatal()
	}
}
