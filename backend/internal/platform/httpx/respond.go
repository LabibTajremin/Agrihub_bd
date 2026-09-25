package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/idgen"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// Request decoding errors.
var (
	ErrMalformed = errs.Validation("request.malformed")
	ErrTooLarge  = errs.Validation("request.too_large")
	ErrBadParam  = errs.Validation("request.bad_param")
)

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// Decode strictly decodes a JSON body into dst and validates it.
func Decode(r *http.Request, dst any, v *validator.Validator) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return ErrTooLarge.Wrap(err)
		}
		return ErrMalformed.Wrap(err)
	}
	return v.Struct(dst)
}

// PathID returns the named path parameter, requiring a canonical UUID.
func PathID(r *http.Request, name string) (string, error) {
	id := r.PathValue(name)
	if !idgen.Valid(id) {
		return "", ErrBadParam.WithField(name, "uuid")
	}
	return id, nil
}

// QueryInt parses an optional integer query parameter bounded to [lo, hi].
func QueryInt(r *http.Request, name string, def, lo, hi int64) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < lo || n > hi {
		return 0, ErrBadParam.WithField(name, "integer out of range")
	}
	return n, nil
}

// QueryFloat parses a required float query parameter bounded to [lo, hi].
func QueryFloat(r *http.Request, name string, lo, hi float64) (float64, error) {
	f, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
	if err != nil || f < lo || f > hi {
		return 0, ErrBadParam.WithField(name, "number out of range")
	}
	return f, nil
}
