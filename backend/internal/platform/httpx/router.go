// Package httpx assembles the HTTP handler tree shared by the Docker server and
// the Vercel function: global middleware, per-route middleware, error mapping.
package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/metrics"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
)

// HandlerFunc is an HTTP handler that returns an error for central mapping.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Rate-limit classes.
const (
	ClassDefault = "default"
	ClassAuth    = "auth"
)

// Route declares one endpoint plus the metadata used for OpenAPI generation.
type Route struct {
	Method     string
	Path       string
	Handler    HandlerFunc
	Permission string // required permission; empty = public
	Class      string // rate-limit class; empty = ClassDefault
	Summary    string
	Tag        string
	Query      any // struct with `query` tags documenting query parameters
	Request    any // request body DTO
	Response   any // success body DTO; nil = no body
	Status     int // success status; 0 = 200
	// MaxBodyBytes overrides Options.MaxBodyBytes (e.g. binary uploads); 0 = default.
	MaxBodyBytes int64
}

// Options configures the router.
type Options struct {
	Logger         *slog.Logger
	Clock          clock.Clock
	IDs            idgen.Generator
	Metrics        metrics.Recorder
	Limiter        ratelimit.Limiter
	Rates          map[string]ratelimit.Rate
	RequestTimeout time.Duration
	CORSOrigins    []string
	TrustProxy     bool
	MaxBodyBytes   int64
	Authn          Authenticator
	Authz          Authorizer
	// After runs once the handler has written its response (e.g. outbox relay).
	After func(r *http.Request)
}

// ErrRouteNotFound is returned for unmatched paths.
var ErrRouteNotFound = errs.NotFound("route.not_found")

// Router is the assembled handler tree.
type Router struct {
	opts    Options
	mux     *http.ServeMux
	routes  []Route
	seen    map[string]bool
	handler http.Handler
}

// NewRouter builds an empty router with the global middleware chain:
// RequestID → RealIP → Recoverer → Logger → CORS → mux.
func NewRouter(o Options) *Router {
	rt := &Router{opts: o, mux: http.NewServeMux(), seen: map[string]bool{}}
	rt.mux.Handle("/", rt.wrap(Route{Path: "/", Handler: func(http.ResponseWriter, *http.Request) error {
		return ErrRouteNotFound
	}}))
	rt.handler = Chain(rt.mux,
		RequestID(o.IDs),
		RealIP(o.TrustProxy),
		Recoverer(o.Logger),
		Logger(o.Logger, o.Clock, o.Metrics),
		CORS(o.CORSOrigins),
	)
	return rt
}

// Mount registers routes; duplicate method+path pairs are rejected.
func (rt *Router) Mount(routes ...Route) error {
	for _, r := range routes {
		pattern := r.Method + " " + r.Path
		if rt.seen[pattern] {
			return fmt.Errorf("httpx: duplicate route %s", pattern)
		}
		rt.seen[pattern] = true
		rt.routes = append(rt.routes, r)
		rt.mux.Handle(pattern, rt.wrap(r))
	}
	return nil
}

// Routes returns the mounted routes in registration order.
func (rt *Router) Routes() []Route { return append([]Route(nil), rt.routes...) }

// ServeHTTP implements http.Handler.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) { rt.handler.ServeHTTP(w, r) }

// wrap applies the per-route chain:
// RateLimit → Timeout → Authenticate → Authorize → handler.
func (rt *Router) wrap(r Route) http.Handler {
	o := rt.opts
	class := r.Class
	if class == "" {
		class = ClassDefault
	}
	pattern := r.Path
	if r.Method != "" {
		pattern = r.Method + " " + r.Path
	}
	limit := o.MaxBodyBytes
	if r.MaxBodyBytes > 0 {
		limit = r.MaxBodyBytes
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req.Body = http.MaxBytesReader(w, req.Body, limit)
		if err := r.Handler(w, req); err != nil {
			WriteError(w, req, o.Logger, err)
		}
		if o.After != nil {
			o.After(req)
		}
	})
	chain := Chain(final,
		RateLimit(o.Limiter, class, o.Rates[class], o.Logger),
		Timeout(o.RequestTimeout),
		Authenticate(o.Authn, o.Logger),
		Authorize(o.Authz, r.Permission, o.Logger),
	)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		info(req.Context()).route = pattern
		chain.ServeHTTP(w, req)
	})
}
