package domain

import "math/bits"

// nanoPerHectare is 1 hectare in 1e-9 m².
const nanoPerHectare = 10_000_000_000_000

// scaleByArea returns round(perHa · areaNano / 1e13) with a 128-bit intermediate.
func scaleByArea(perHa, areaNano int64) int64 {
	if perHa <= 0 || areaNano <= 0 {
		return 0
	}
	hi, lo := bits.Mul64(uint64(perHa), uint64(areaNano))
	lo, carry := bits.Add64(lo, nanoPerHectare/2, 0)
	q, _ := bits.Div64(hi+carry, lo, nanoPerHectare)
	return int64(q) //nolint:gosec // bounded: field ≤ 1e5 ha, per-ha values ≤ 1e9
}

// EstimateYield scales per-hectare yields to the field area (kg).
func EstimateYield(perHa Yield, areaNano int64) Yield {
	return Yield{scaleByArea(perHa.Low, areaNano), scaleByArea(perHa.Likely, areaNano), scaleByArea(perHa.High, areaNano)}
}

// DefaultCosts scales a crop's per-hectare costs (poisha) to the field area.
func DefaultCosts(perHa map[string]int64, areaNano int64) map[string]int64 {
	out := make(map[string]int64, len(perHa))
	for k, v := range perHa {
		out[k] = scaleByArea(v, areaNano)
	}
	return out
}

// ROI is a return-on-investment estimate in poisha (integer money, §6.6).
type ROI struct {
	Gross     Yield // income: low / likely / high
	TotalCost int64
	Net       Yield // gross − total cost
	// ReturnBP is likely net / total cost in basis points (0 when cost is 0).
	ReturnBP int64
}

// EstimateROI prices a yield triple and subtracts costs. Pure integer
// arithmetic: net = gross − Σcosts exactly.
func EstimateROI(yield Yield, pricePoishaPerKg int64, costs map[string]int64) ROI {
	var total int64
	for _, c := range costs {
		total += c
	}
	gross := Yield{yield.Low * pricePoishaPerKg, yield.Likely * pricePoishaPerKg, yield.High * pricePoishaPerKg}
	r := ROI{Gross: gross, TotalCost: total, Net: Yield{gross.Low - total, gross.Likely - total, gross.High - total}}
	if total > 0 {
		r.ReturnBP = r.Net.Likely * 10000 / total
	}
	return r
}
