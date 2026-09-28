package app

import (
	"context"
	"io"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/domain"
	diag "github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather"
)

// The composition root is the only place modules meet. Each adapter
// implements a consumer's port (declared in the consumer's domain) on top of a
// provider module's published view. Extracting a module into a service turns
// the matching adapter into an HTTP/gRPC client; no other code changes.

// diagnosisMedia implements diagnosis/domain.Media on the media module.
type diagnosisMedia struct{ m *media.Module }

func (a diagnosisMedia) Info(ctx context.Context, id string) (diag.MediaInfo, error) {
	o, err := a.m.Object(ctx, id)
	return diag.MediaInfo{OwnerID: o.OwnerID, ContentType: o.ContentType, PHash: o.PHash, Uploaded: o.Uploaded}, err
}

func (a diagnosisMedia) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	return a.m.Open(ctx, id)
}

// advisoryFarm implements advisory's field and crop sources on the farm module.
type advisoryFarm struct{ m *farm.Module }

func (a advisoryFarm) Field(ctx context.Context, id string) (domain.Field, error) {
	v, err := a.m.Field(ctx, id)
	f := domain.Field{ID: v.ID, OwnerID: v.OwnerID, Lat: v.Lat, Lng: v.Lng, AreaNano: v.AreaNano, Irrigation: v.Irrigation}
	if v.Soil != nil {
		f.Texture, f.PHx10, f.NitrogenKgHa = v.Soil.Texture, v.Soil.PHx10, v.Soil.NitrogenKgHa
	}
	if len(v.CropCodes) > 0 {
		f.CurrentCrop = v.CropCodes[len(v.CropCodes)-1]
	}
	return f, err
}

func (a advisoryFarm) Crops() []domain.Crop {
	views := a.m.Crops()
	out := make([]domain.Crop, len(views))
	for i, c := range views {
		out[i] = domain.Crop{Code: c.Code, Seasons: c.Seasons, WaterNeedMM: c.WaterNeedMM, Textures: c.Textures, PHMinX10: c.PHMinX10,
			PHMaxX10: c.PHMaxX10, NitrogenDeltaKgHa: c.NitrogenDeltaKgHa, YieldKgHa: domain.Yield{Low: c.YieldLow, Likely: c.YieldLikely, High: c.YieldHigh},
			PricePoishaPerKg: c.PricePoishaPerKg, SeedAvailBP: c.SeedAvailBP, MarketDemandBP: c.MarketDemandBP, PestRiskBP: c.PestRiskBP,
			CostPoishaPerHa: c.CostPoishaPerHa}
	}
	return out
}

// advisoryWeather implements advisory's weather source on the weather module.
type advisoryWeather struct{ m *weather.Module }

func (a advisoryWeather) Outlook(ctx context.Context, lat, lng float64) (domain.Outlook, error) {
	o, err := a.m.Outlook(ctx, lat, lng)
	return domain.Outlook{RainMM: o.RainMM, AgeSeconds: o.AgeSeconds, Stale: o.Stale}, err
}
