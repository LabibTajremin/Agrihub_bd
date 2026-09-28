// Package transport exposes weather over HTTP.
package transport

import (
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
)

// PointQuery locates a forecast.
type PointQuery struct {
	Lat float64 `query:"lat"`
	Lng float64 `query:"lng"`
}

// Day is one forecast day in display units.
type Day struct {
	Date        string  `json:"date"`
	TempMinC    float64 `json:"temp_min_c"`
	TempMaxC    float64 `json:"temp_max_c"`
	RainMM      float64 `json:"rain_mm"`
	HumidityPct int     `json:"humidity_pct"`
	WindKph     int     `json:"wind_kph"`
}

// Forecast is a forecast with staleness metadata (the UI shows data_age_seconds).
type Forecast struct {
	CellKey        string    `json:"cell_key"`
	FetchedAt      time.Time `json:"fetched_at"`
	DataAgeSeconds int64     `json:"data_age_seconds"`
	Stale          bool      `json:"stale"`
	Days           []Day     `json:"days"`
}

// Outlook is the seasonal rainfall outlook.
type Outlook struct {
	CurrentSeason  string         `json:"current_season"`
	RainMM         map[string]int `json:"rain_mm"`
	DataAgeSeconds int64          `json:"data_age_seconds"`
	Stale          bool           `json:"stale"`
}

// Handlers groups the weather endpoints.
type Handlers struct {
	Forecast usecase.GetForecast
	Outlook  usecase.GetOutlook
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/v1/weather", Permission: string(authz.WeatherRead), Tag: "weather", Summary: "7-day forecast with data age",
			Query: PointQuery{}, Response: Forecast{}, Handler: h.forecast},
		{Method: http.MethodGet, Path: "/v1/weather/seasonal", Permission: string(authz.WeatherRead), Tag: "weather", Summary: "Seasonal rainfall outlook",
			Query: PointQuery{}, Response: Outlook{}, Handler: h.outlook},
	}
}

func point(r *http.Request) (float64, float64, error) {
	lat, err := httpx.QueryFloat(r, "lat", -90, 90)
	if err != nil {
		return 0, 0, err
	}
	lng, err := httpx.QueryFloat(r, "lng", -180, 180)
	return lat, lng, err
}

// ToForecast renders a report.
func ToForecast(rep domain.Report) Forecast {
	out := Forecast{CellKey: rep.Cell.Key(), FetchedAt: rep.FetchedAt, DataAgeSeconds: rep.AgeSeconds, Stale: rep.Stale,
		Days: make([]Day, len(rep.Forecast.Days))}
	for i, d := range rep.Forecast.Days {
		out.Days[i] = Day{Date: d.Date, TempMinC: float64(d.TempMinC10) / 10, TempMaxC: float64(d.TempMaxC10) / 10,
			RainMM: float64(d.RainMM10) / 10, HumidityPct: d.HumidityPct, WindKph: d.WindKph}
	}
	return out
}

func (h Handlers) forecast(w http.ResponseWriter, r *http.Request) error {
	lat, lng, err := point(r)
	if err != nil {
		return err
	}
	rep, err := h.Forecast.Execute(r.Context(), lat, lng)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, ToForecast(rep))
}

func (h Handlers) outlook(w http.ResponseWriter, r *http.Request) error {
	lat, lng, err := point(r)
	if err != nil {
		return err
	}
	o, err := h.Outlook.Execute(r.Context(), lat, lng) // only fails on authorization, which middleware checks first
	if err == nil {
		err = httpx.JSON(w, http.StatusOK, Outlook{CurrentSeason: o.Season, RainMM: o.RainMM, DataAgeSeconds: o.AgeSeconds, Stale: o.Stale})
	}
	return err
}
