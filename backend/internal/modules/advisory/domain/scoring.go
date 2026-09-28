package domain

import (
	"container/heap"
	"math"
	"slices"
	"sort"
)

// Criteria are the normalised criterion values, each in [0,1].
type Criteria struct {
	Soil, Water, Pest, Market, Seed float64
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// soilScore blends texture match (60%) and pH fit (40%). Unknown soil data
// scores neutrally rather than zero.
func soilScore(f Field, c Crop) float64 {
	texture := 0.4
	if f.Texture != "" {
		texture = 0
		if slices.Contains(c.Textures, f.Texture) {
			texture = 1
		}
	}
	ph := 0.5
	if f.PHx10 > 0 {
		dist := max(c.PHMinX10-f.PHx10, f.PHx10-c.PHMaxX10, 0)
		ph = clamp01(1 - float64(dist)/15) // 1.5 pH units outside the range scores 0
	}
	return 0.6*texture + 0.4*ph
}

// irrigationShare is the fraction of the crop's water need supplied by irrigation.
func irrigationShare(level string) float64 {
	switch level {
	case "full":
		return 1
	case "partial":
		return 0.5
	default:
		return 0
	}
}

// waterScore compares available water (rain + irrigation) with need and
// penalises waterlogging when rain alone exceeds twice the need.
func waterScore(f Field, c Crop, rainMM int) float64 {
	need := float64(max(c.WaterNeedMM, 1))
	rain := float64(max(rainMM, 0))
	s := clamp01((rain + irrigationShare(f.Irrigation)*need) / need)
	if rain > 2*need {
		s *= clamp01(1 - (rain-2*need)/(2*need))
	}
	return s
}

// Normalise maps field, crop and forecast into criteria in [0,1].
func Normalise(f Field, c Crop, rainMM int) Criteria {
	return Criteria{
		Soil:   soilScore(f, c),
		Water:  waterScore(f, c, rainMM),
		Pest:   clamp01(1 - float64(c.PestRiskBP)/10000),
		Market: clamp01(float64(c.MarketDemandBP) / 10000),
		Seed:   clamp01(float64(c.SeedAvailBP) / 10000),
	}
}

// Score is Σ(wᵢ·criterionᵢ)·100, in [0,100] for valid weights, rounded to 0.01.
func Score(c Criteria, w Weights) float64 {
	s := 100 * (w.Soil*c.Soil + w.Water*c.Water + w.Pest*c.Pest + w.Market*c.Market + w.Seed*c.Seed)
	return math.Round(s*100) / 100
}

// Scored is a ranked candidate.
type Scored struct {
	Crop     Crop
	Criteria Criteria
	Score    float64
}

// better orders by score descending, then crop code ascending (deterministic ties).
func better(a, b Scored) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Crop.Code < b.Crop.Code
}

// worstFirst is a min-heap whose root is the weakest kept candidate.
type worstFirst []Scored

func (h worstFirst) Len() int           { return len(h) }
func (h worstFirst) Less(i, j int) bool { return better(h[j], h[i]) }
func (h worstFirst) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *worstFirst) Push(x any)        { *h = append(*h, x.(Scored)) }
func (h *worstFirst) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// TopN selects the n best candidates in O(len·log n) with a bounded heap,
// returned best first with deterministic tie-breaking by crop code.
func TopN(all []Scored, n int) []Scored {
	if n <= 0 {
		return nil
	}
	h := make(worstFirst, 0, n+1)
	for _, s := range all {
		if h.Len() < n {
			heap.Push(&h, s)
			continue
		}
		if better(s, h[0]) {
			h[0] = s
			heap.Fix(&h, 0)
		}
	}
	out := []Scored(h)
	sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
	return out
}

// Rank scores every crop grown in season and returns the top n.
func Rank(f Field, crops []Crop, season string, rainMM int, w Weights, n int) []Scored {
	var all []Scored
	for _, c := range crops {
		if !slices.Contains(c.Seasons, season) {
			continue
		}
		cr := Normalise(f, c, rainMM)
		all = append(all, Scored{Crop: c, Criteria: cr, Score: Score(cr, w)})
	}
	return TopN(all, n)
}
