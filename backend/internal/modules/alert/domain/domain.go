// Package domain holds the alert model and the pure rules engine that turns
// weather and diagnosis events into alerts.
package domain

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Alert kinds.
const (
	KindHeavyRain       = "heavy_rain"
	KindHeat            = "heat"
	KindBlastRisk       = "blast_risk"
	KindDiseaseFollowup = "disease_followup"
)

// Kinds lists every alert kind.
func Kinds() []string { return []string{KindHeavyRain, KindHeat, KindBlastRisk, KindDiseaseFollowup} }

// Severities.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

// Errors.
var (
	ErrNotFound            = errs.NotFound("alert.not_found")
	ErrInvalidSubscription = errs.Validation("alert.invalid_subscription")
)

// Errors lists the alert error catalogue.
func Errors() []*errs.Error { return []*errs.Error{ErrNotFound, ErrInvalidSubscription} }

// Alert is one notification for one user. Title and body are dictionary keys.
type Alert struct {
	ID            string
	UserID        string
	Kind          string
	Severity      string
	TitleKey      string
	BodyKey       string
	Params        map[string]string
	SourceEventID string
	CreatedAt     time.Time
	ReadAt        *time.Time
}

// Subscription registers a user's location for area alerts.
type Subscription struct {
	UserID    string
	CellKey   string
	Lat, Lng  float64
	Kinds     []string
	UpdatedAt time.Time
}

// CellKey is the 0.1° cell of a point (same grid as the weather module).
func CellKey(lat, lng float64) string {
	return fmt.Sprintf("%.1f:%.1f", math.Round(lat*10)/10, math.Round(lng*10)/10)
}

// Candidate is an alert a rule wants to raise.
type Candidate struct {
	Kind     string
	Severity string
	Params   map[string]string
}

// Day is the part of a forecast day the rules read (tenths of °C / mm).
type Day struct {
	TempMinC10  int `json:"temp_min_c10"`
	TempMaxC10  int `json:"temp_max_c10"`
	RainMM10    int `json:"rain_mm10"`
	HumidityPct int `json:"humidity_pct"`
}

// Rule thresholds.
const (
	HeavyRainMM10    = 500  // 50 mm/day warning
	ExtremeRainMM10  = 1000 // 100 mm/day critical
	HeatC10          = 350  // 35 °C warning
	ExtremeHeatC10   = 380  // 38 °C critical
	BlastHumidityPct = 85
	BlastMinC10      = 200 // warm nights 20–26 °C with high humidity favour rice blast
	BlastMaxC10      = 260
)

// EvaluateForecast applies the weather rules to a forecast: one candidate per
// triggered kind, carrying the worst day's value.
func EvaluateForecast(days []Day) []Candidate {
	var rain, heat int
	blast := false
	for _, d := range days {
		rain = max(rain, d.RainMM10)
		heat = max(heat, d.TempMaxC10)
		blast = blast || (d.HumidityPct >= BlastHumidityPct && d.TempMinC10 >= BlastMinC10 && d.TempMinC10 <= BlastMaxC10)
	}
	var out []Candidate
	if rain >= HeavyRainMM10 {
		out = append(out, Candidate{KindHeavyRain, severity(rain >= ExtremeRainMM10), map[string]string{"mm": strconv.Itoa(rain / 10)}})
	}
	if heat >= HeatC10 {
		out = append(out, Candidate{KindHeat, severity(heat >= ExtremeHeatC10), map[string]string{"temp": strconv.Itoa(heat / 10)}})
	}
	if blast {
		out = append(out, Candidate{KindBlastRisk, SeverityWarning, map[string]string{}})
	}
	return out
}

func severity(critical bool) string {
	if critical {
		return SeverityCritical
	}
	return SeverityWarning
}

// EvaluateScan raises a follow-up reminder for a diagnosed disease.
func EvaluateScan(status, disease string) []Candidate {
	if status != "completed" || disease == "" || disease == "healthy" {
		return nil
	}
	return []Candidate{{KindDiseaseFollowup, SeverityInfo, map[string]string{"disease": "disease." + disease + ".name"}}}
}

// Build turns a candidate into an alert for a user.
func (c Candidate) Build(id, userID, sourceEventID string, at time.Time) Alert {
	return Alert{ID: id, UserID: userID, Kind: c.Kind, Severity: c.Severity, TitleKey: "alerts." + c.Kind + ".title",
		BodyKey: "alerts." + c.Kind + ".body", Params: c.Params, SourceEventID: sourceEventID, CreatedAt: at}
}

// Alerts is the alert repository port.
type Alerts interface {
	// Insert stores a, returning false if (user, source event, kind) exists.
	Insert(ctx context.Context, a Alert) (bool, error)
	Get(ctx context.Context, id string) (Alert, error)
	List(ctx context.Context, userID string, unreadOnly bool, limit int) ([]Alert, error)
	MarkRead(ctx context.Context, id string, at time.Time) error
	MarkAllRead(ctx context.Context, userID string, at time.Time) (int, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
}

// Subscriptions is the subscription repository port.
type Subscriptions interface {
	Put(ctx context.Context, s Subscription) error
	Get(ctx context.Context, userID string) (Subscription, error)
	Delete(ctx context.Context, userID string) error
	InCell(ctx context.Context, cellKey string) ([]Subscription, error)
}

// Notifier pushes an alert to a device. MVP ships a logging stub (§14.5).
type Notifier interface {
	Notify(ctx context.Context, a Alert) error
}
