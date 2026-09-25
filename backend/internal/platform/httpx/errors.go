package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/logger"
)

// Status maps an error kind to its HTTP status. This is the only such mapping.
func Status(k errs.Kind) int {
	switch k {
	case errs.KindValidation:
		return http.StatusBadRequest
	case errs.KindNotFound:
		return http.StatusNotFound
	case errs.KindConflict:
		return http.StatusConflict
	case errs.KindUnauthorized:
		return http.StatusUnauthorized
	case errs.KindForbidden:
		return http.StatusForbidden
	case errs.KindRateLimited:
		return http.StatusTooManyRequests
	case errs.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// ErrorBody is the wire format shared by every endpoint.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries the machine code, the i18n message key and field details.
type ErrorDetail struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id"`
}

// WriteError renders err. Internal causes are logged, never sent to the client.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	e := errs.From(err)
	status := Status(e.Kind)
	detail := ErrorDetail{Code: e.Code, Message: e.Message, Fields: e.Fields, RequestID: logger.RequestID(r.Context())}
	if status == http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "internal error", "error", err.Error(), "path", r.URL.Path)
		detail = ErrorDetail{Code: "internal", Message: "errors.internal", RequestID: detail.RequestID}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: detail})
}
