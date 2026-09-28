// Package app is the composition root: it turns a validated Config into the
// single http.Handler tree served by both deployment targets (the Docker
// server in cmd/api and the Vercel function in api/index.go).
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory"
	advdomain "github.com/labibtajremin/agrihub_bd/backend/internal/modules/advisory/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/assistant"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/aiadapter/registry"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/alert"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/diagnosis"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/farm"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity"
	idusecase "github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/media"
	mediausecase "github.com/labibtajremin/agrihub_bd/backend/internal/modules/media/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/weather"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/config"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/kernel"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/metrics"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/outbox"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// DiagnosisCacheSize bounds the dHash → diagnosis LRU.
const DiagnosisCacheSize = 4096

// Infra holds the process-level dependencies, injectable for tests.
type Infra struct {
	Clock     clock.Clock
	IDs       idgen.Generator
	Random    io.Reader
	LogOutput io.Writer
}

// DefaultInfra is the production infrastructure.
func DefaultInfra() Infra {
	return Infra{Clock: clock.System{}, IDs: idgen.UUIDv7{}, Random: rand.Reader, LogOutput: os.Stdout}
}

// App is the assembled application.
type App struct {
	Config       *config.Config
	Logger       *slog.Logger
	Router       *httpx.Router
	DB           *database.DB
	Relay        *outbox.Relay
	Metrics      *metrics.Registry
	Localization *localization.Module
	Signer       *authn.Signer
	clock        clock.Clock
	closers      []func()
}

// Close releases pools and clients.
func (a *App) Close() {
	for _, c := range a.closers {
		c()
	}
}

// Build wires every module over the configured infrastructure.
func Build(ctx context.Context, cfg *config.Config, in Infra) (*App, error) {
	log := logger.New(in.LogOutput, cfg.Observability.LogLevel, cfg.Observability.LogFormat)
	db, pool, err := database.Open(ctx, database.Options{URL: cfg.Database.URL, PoolMode: cfg.Database.PoolMode,
		MaxConns: int32(cfg.Database.MaxConns), MinConns: int32(cfg.Database.MinConns), ConnectTimeout: cfg.Database.ConnectTimeout}) //nolint:gosec // validated small ints
	if err != nil {
		return nil, err
	}
	a := &App{Config: cfg, Logger: log, DB: db, Metrics: metrics.NewRegistry(), clock: in.Clock, closers: []func(){pool.Close}}
	limiter, err := a.limiter(cfg.Redis.URL, in.Clock)
	if err != nil {
		a.Close()
		return nil, err
	}
	bus := eventbus.NewLocal()
	k := kernel.Kernel{DB: db, Clock: in.Clock, IDs: in.IDs, Validator: validator.New(), Logger: log, Events: outbox.NewPublisher(db), Bus: bus}
	a.Relay = outbox.NewRelay(db, bus, in.Clock, cfg.Database.OutboxBatchSize)
	if err := a.mount(cfg, k, in, limiter); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) limiter(redisURL string, c clock.Clock) (ratelimit.Limiter, error) {
	memory := ratelimit.NewMemory(c)
	if redisURL == "" {
		return memory, nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	a.closers = append(a.closers, func() { _ = client.Close() })
	return ratelimit.Fallback{Primary: ratelimit.NewRedis(client, c), Secondary: memory, Logger: a.Logger}, nil
}

func (a *App) mount(cfg *config.Config, k kernel.Kernel, in Infra, limiter ratelimit.Limiter) error {
	signer := authn.NewSigner(authn.SignerOptions{Keys: map[string][]byte{cfg.Auth.JWTKeyID: []byte(cfg.Auth.JWTSecret)},
		ActiveKID: cfg.Auth.JWTKeyID, Issuer: cfg.Auth.Issuer, TTL: cfg.Auth.AccessTTL, Clock: in.Clock, IDs: in.IDs})
	store, local, errStorage := media.NewStorage(cfg.Storage, cfg.App.BaseURL, in.Clock)
	ai, errAI := registry.New(cfg.AI.Provider)

	idm, e1 := identity.New(identity.Deps{Kernel: k, Signer: signer, Limiter: limiter, Random: in.Random,
		Hasher: authn.NewArgon2(authn.Argon2Params{Time: cfg.Auth.Argon2Time, Memory: cfg.Auth.Argon2MemoryKiB,
			Threads: cfg.Auth.Argon2Threads, KeyLen: cfg.Auth.Argon2KeyLen}, in.Random),
		Policy: idusecase.Policy{OTPTTL: cfg.Auth.OTPTTL, RefreshTTL: cfg.Auth.RefreshTTL, OTPLength: cfg.Auth.OTPLength,
			OTPMaxAttempts: cfg.Auth.OTPMaxAttempts, OTPPerNumber: cfg.Auth.OTPPerNumberPerHour, OTPPerIP: cfg.Auth.OTPPerIPPerHour,
			GuestMode: cfg.Features.GuestMode, ExposeOTP: cfg.Features.ExposeOTP}})
	loc, e2 := localization.New(localization.Deps{Kernel: k, CacheMaxAge: cfg.Localization.CacheMaxAge})
	fm, e3 := farm.New(farm.Deps{Kernel: k})
	mm, e4 := media.New(media.Deps{Kernel: k, Storage: store, Local: local, Policy: mediausecase.Policy{
		AllowedTypes: cfg.Storage.AllowedContentTypes, MaxBytes: cfg.Storage.MaxUploadBytes, PresignTTL: cfg.Storage.PresignTTL}})
	wm, e5 := weather.New(weather.Deps{Kernel: k, Config: cfg.Weather})
	dm, e6 := diagnosis.New(diagnosis.Deps{Kernel: k, Engine: ai.Diagnosis, Media: diagnosisMedia{mm}, MinConfidence: cfg.AI.MinDiagnosisConfidence,
		CacheSize: DiagnosisCacheSize, MaxImageBytes: cfg.Storage.MaxUploadBytes})
	am, e7 := advisory.New(advisory.Deps{Kernel: k, Fields: advisoryFarm{fm}, Crops: advisoryFarm{fm}, Weather: advisoryWeather{wm},
		Narrator: ai.Narrator, TopN: cfg.Advisory.TopN, Weights: advdomain.Weights{Soil: cfg.Advisory.WeightSoil, Water: cfg.Advisory.WeightWater,
			Pest: cfg.Advisory.WeightPest, Market: cfg.Advisory.WeightMarket, Seed: cfg.Advisory.WeightSeed}})
	al, e8 := alert.New(alert.Deps{Kernel: k})
	if err := errors.Join(errStorage, errAI, e1, e2, e3, e4, e5, e6, e7, e8); err != nil {
		return err
	}
	a.Localization, a.Signer = loc, signer

	h := authn.HTTP{Verifier: signer}
	a.Router = httpx.NewRouter(httpx.Options{
		Logger: a.Logger, Clock: in.Clock, IDs: in.IDs, Metrics: a.Metrics, Limiter: limiter,
		Rates: map[string]ratelimit.Rate{
			httpx.ClassDefault: ratelimit.PerMinute(cfg.HTTP.RateLimitPerMinute),
			httpx.ClassAuth:    ratelimit.PerMinute(cfg.HTTP.AuthRateLimitPerMinute),
		},
		RequestTimeout: cfg.HTTP.RequestTimeout, CORSOrigins: cfg.HTTP.CORSAllowedOrigins, TrustProxy: cfg.HTTP.TrustProxyHeaders,
		MaxBodyBytes: cfg.HTTP.MaxBodyBytes, Authn: h, Authz: h, After: a.flushAfterWrite,
	})
	var routes []httpx.Route
	routes = append(routes, a.platformRoutes()...)
	for _, rs := range [][]httpx.Route{idm.Routes(), loc.Routes(), fm.Routes(), mm.Routes(), wm.Routes(), dm.Routes(), am.Routes(),
		al.Routes(), assistant.New(ai.Agent, k.Validator).Routes()} {
		routes = append(routes, rs...)
	}
	return a.Router.Mount(routes...)
}

// flushAfterWrite relays outbox events after each request (reads can publish
// too, e.g. a fresh weather fetch), so events flow on serverless where there
// is no background worker. With nothing pending it is one indexed query.
func (a *App) flushAfterWrite(r *http.Request) {
	if _, err := a.Relay.Flush(context.WithoutCancel(r.Context())); err != nil {
		a.Logger.WarnContext(r.Context(), "outbox flush failed", "error", err)
	}
}

// Health is the liveness/readiness body.
type Health struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

// MetricsSnapshot lists request metrics.
type MetricsSnapshot struct {
	Series []metrics.Series `json:"series"`
}

func (a *App) platformRoutes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/healthz", Tag: "platform", Summary: "Liveness", Response: Health{}, Handler: func(w http.ResponseWriter, _ *http.Request) error {
			return httpx.JSON(w, http.StatusOK, Health{Status: "ok", Time: a.clock.Now()})
		}},
		{Method: http.MethodGet, Path: "/readyz", Tag: "platform", Summary: "Readiness (database reachable)", Response: Health{}, Handler: a.ready},
		{Method: http.MethodGet, Path: "/v1/admin/metrics", Permission: string(authz.MetricsRead), Tag: "platform", Summary: "Request metrics",
			Response: MetricsSnapshot{}, Handler: func(w http.ResponseWriter, _ *http.Request) error {
				return httpx.JSON(w, http.StatusOK, MetricsSnapshot{Series: a.Metrics.Snapshot()})
			}},
	}
}

var errNotReady = errors.New("database not reachable")

func (a *App) ready(w http.ResponseWriter, r *http.Request) error {
	var one int
	if err := a.DB.Q(r.Context()).QueryRow(r.Context(), "SELECT 1").Scan(&one); err != nil {
		return httpx.JSON(w, http.StatusServiceUnavailable, Health{Status: errNotReady.Error(), Time: a.clock.Now()})
	}
	return httpx.JSON(w, http.StatusOK, Health{Status: "ready", Time: a.clock.Now()})
}
