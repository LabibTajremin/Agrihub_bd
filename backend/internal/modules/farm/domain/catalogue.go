package domain

import "sort"

// Seasons (Bangladesh cropping calendar).
const (
	SeasonAman = "aman" // monsoon, Jul–Nov
	SeasonBoro = "boro" // dry winter (rabi), Nov–May
	SeasonAus  = "aus"  // pre-monsoon (kharif-1), Mar–Jul
)

// Seasons lists seasons in calendar order (aman → boro → aus → aman …).
func Seasons() []string { return []string{SeasonAman, SeasonBoro, SeasonAus} }

// NextSeason returns the season that follows s.
func NextSeason(s string) string {
	ss := Seasons()
	for i, x := range ss {
		if x == s {
			return ss[(i+1)%len(ss)]
		}
	}
	return ""
}

// Yield is a low / likely / high triple in kg per hectare.
type Yield struct{ Low, Likely, High int64 }

// Crop is a catalogue entry. Fractions are basis points (0–10000) and money is
// poisha (1/100 taka) so the catalogue carries no floats.
type Crop struct {
	Code              string
	Seasons           []string
	WaterNeedMM       int
	Textures          []Texture
	PHMinX10          int
	PHMaxX10          int
	NitrogenDeltaKgHa int // negative consumes soil N, positive fixes it (legumes)
	DurationDays      int
	YieldKgHa         Yield
	PricePoishaPerKg  int64
	SeedAvailBP       int
	MarketDemandBP    int
	PestRiskBP        int
	CostPoishaPerHa   map[string]int64
}

func costs(seed, fert, labour, irr, pest int64) map[string]int64 {
	return map[string]int64{"seed": seed * 100, "fertiliser": fert * 100, "labour": labour * 100, "irrigation": irr * 100, "pesticide": pest * 100}
}

var (
	riceSoils  = []Texture{Clay, ClayLoam, SiltLoam}
	uplandMix  = []Texture{Loam, SandyLoam, SiltLoam}
	pulseSoils = []Texture{Loam, ClayLoam, SandyLoam}
)

// Catalogue returns the crop catalogue sorted by code (fresh copy; the data is
// agronomic reference values for Bangladesh, per hectare, in taka ×100).
func Catalogue() []Crop {
	cs := []Crop{
		{"rice_aman", []string{SeasonAman}, 1100, riceSoils, 50, 70, -90, 140, Yield{3000, 4500, 5500}, 3000, 9000, 8000, 4000, costs(3000, 12000, 25000, 2000, 4000)},
		{"rice_boro", []string{SeasonBoro}, 1200, riceSoils, 50, 70, -110, 150, Yield{4500, 6000, 7000}, 3000, 9000, 8500, 4500, costs(3500, 15000, 30000, 15000, 5000)},
		{"rice_aus", []string{SeasonAus}, 800, []Texture{ClayLoam, Loam, SiltLoam}, 50, 75, -70, 110, Yield{2500, 3500, 4500}, 2800, 7000, 7000, 3500, costs(2500, 9000, 20000, 3000, 3000)},
		{"wheat", []string{SeasonBoro}, 350, []Texture{Loam, ClayLoam, SiltLoam, SandyLoam}, 60, 75, -80, 110, Yield{2500, 3500, 4200}, 3500, 8000, 7000, 3000, costs(6000, 12000, 18000, 4000, 2000)},
		{"maize", []string{SeasonBoro, SeasonAus}, 500, uplandMix, 55, 75, -120, 120, Yield{6000, 8000, 10000}, 2500, 8500, 8000, 3000, costs(9000, 18000, 20000, 6000, 3000)},
		{"jute", []string{SeasonAus}, 600, []Texture{Loam, ClayLoam, SiltLoam}, 60, 75, -40, 120, Yield{2000, 2800, 3500}, 6000, 7500, 6000, 2500, costs(2000, 8000, 30000, 2000, 2000)},
		{"potato", []string{SeasonBoro}, 450, []Texture{SandyLoam, Loam}, 50, 65, -100, 90, Yield{18000, 24000, 30000}, 1800, 8000, 7500, 5000, costs(60000, 25000, 30000, 8000, 8000)},
		{"lentil", []string{SeasonBoro}, 250, pulseSoils, 60, 75, 40, 110, Yield{900, 1300, 1700}, 10000, 7000, 8000, 2000, costs(4000, 4000, 12000, 0, 2000)},
		{"mustard", []string{SeasonBoro}, 300, []Texture{Loam, SandyLoam, SiltLoam, ClayLoam}, 60, 75, -60, 90, Yield{1000, 1400, 1800}, 9000, 8000, 7500, 2500, costs(1500, 8000, 10000, 2000, 2000)},
		{"mungbean", []string{SeasonAus}, 300, uplandMix, 62, 72, 35, 65, Yield{800, 1100, 1400}, 12000, 7000, 7500, 3000, costs(3000, 3000, 10000, 1000, 2000)},
		{"onion", []string{SeasonBoro}, 400, uplandMix, 60, 70, -70, 120, Yield{10000, 14000, 18000}, 4000, 6000, 9000, 4500, costs(20000, 15000, 35000, 6000, 5000)},
		{"tomato", []string{SeasonBoro}, 450, []Texture{Loam, SandyLoam}, 60, 70, -90, 110, Yield{25000, 35000, 45000}, 2500, 7000, 8500, 5500, costs(8000, 20000, 40000, 8000, 10000)},
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Code < cs[j].Code })
	return cs
}

// FindCrop returns the crop with code.
func FindCrop(code string) (Crop, error) {
	for _, c := range Catalogue() {
		if c.Code == code {
			return c, nil
		}
	}
	return Crop{}, ErrCropNotFound
}
