package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/clock"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/metrics"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies mws so the first one is outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// HeaderRequestID carries the request ID in both directions.
const HeaderRequestID = "X-Request-ID"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID accepts a well-formed inbound X-Request-ID or generates one, stores
// it in the context and echoes it on the response.
func RequestID(ids idgen.Generator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			if !requestIDPattern.MatchString(id) {
				id = ids.New()
			}
			w.Header().Set(HeaderRequestID, id)
			ctx := withInfo(logger.WithRequestID(r.Context(), id), &reqInfo{})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RealIP resolves the client IP, honouring proxy headers only when trusted.
func RealIP(trustProxy bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info(r.Context()).clientIP = resolveIP(r, trustProxy)
			next.ServeHTTP(w, r)
		})
	}
}

func resolveIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Recoverer converts a panic into a logged 500 so one bad request cannot crash
// the process. It is a safety net, not control flow.
func Recoverer(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					WriteError(w, r, log, errs.Internal("panic").Wrap(fmt.Errorf("%v", v)))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Logger writes one structured access-log line per request and records metrics.
func Logger(log *slog.Logger, c clock.Clock, m metrics.Recorder) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := c.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			d := c.Now().Sub(start)
			i := info(r.Context())
			route := i.route
			if route == "" {
				route = "unmatched"
			}
			m.ObserveRequest(route, rec.status, d)
			log.InfoContext(r.Context(), "http request",
				"method", r.Method, "route", route, "status", rec.status,
				"duration_ms", d.Milliseconds(), "bytes", rec.bytes,
				"client_ip", i.clientIP, "subject", i.subject)
		})
	}
}

// CORS applies the allow-list and answers preflight requests.
func CORS(origins []string) Middleware {
	any := slices.Contains(origins, "*")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			h := w.Header()
			h.Add("Vary", "Origin")
			allowed := origin != "" && (any || slices.Contains(origins, origin))
			if allowed {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Expose-Headers", "X-Request-ID, ETag, Retry-After")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if allowed {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, If-None-Match, X-Request-ID, Idempotency-Key")
					h.Set("Access-Control-Max-Age", "600")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ErrRateLimited is returned when a bucket is empty.
var ErrRateLimited = errs.RateLimited("request.rate_limited")

// RateLimit takes a token from the (principal, route-class) bucket. Before
// authentication the principal is identified by a hash of its bearer token,
// falling back to the client IP. Limiter failures fail open.
func RateLimit(l ratelimit.Limiter, class string, rate ratelimit.Rate, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d, err := l.Allow(r.Context(), class+":"+principalKey(r), rate)
			if err != nil {
				log.WarnContext(r.Context(), "rate limiter unavailable; allowing", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if !d.Allowed {
				secs := int(d.RetryAfter.Round(time.Second) / time.Second)
				w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
				WriteError(w, r, log, ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func principalKey(r *http.Request) string {
	if tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && tok != "" {
		sum := sha256.Sum256([]byte(tok))
		return "tok:" + hex.EncodeToString(sum[:8])
	}
	return "ip:" + ClientIP(r.Context())
}

// Timeout bounds the request context.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authenticator resolves the caller from the request and returns a context
// carrying the principal (anonymous when no credentials are presented).
type Authenticator interface {
	Authenticate(r *http.Request) (context.Context, error)
}

// Authorizer fast-fails requests lacking a permission. Use cases re-check.
type Authorizer interface {
	Authorize(ctx context.Context, permission string) error
}

// Authenticate runs the Authenticator.
func Authenticate(a Authenticator, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, err := a.Authenticate(r)
			if err != nil {
				WriteError(w, r, log, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authorize requires permission (skipped when empty: public route).
func Authorize(a Authorizer, permission string, log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		if permission == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := a.Authorize(r.Context(), permission); err != nil {
				WriteError(w, r, log, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
