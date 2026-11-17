// Package httpapi contains the HTTP handlers and the router wiring.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/featureflag/api/internal/domain"
)

// errorResponse is the stable error envelope returned by every endpoint.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes returned by the API. They are stable and safe to expose;
// they never include SQL errors or internal detail.
const (
	CodeValidation   = "validation_failed"
	CodeUnauthorized = "unauthorized"
	CodeForbidden    = "forbidden"
	CodeNotFound     = "not_found"
	CodeConflict     = "conflict"
	CodeInternal     = "internal_error"
	CodeRateLimited  = "rate_limited"
)

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write json response", "error", err)
	}
}

// writeError maps a domain error to a stable HTTP response.
//
// The mapping is the only place that turns internal errors into status
// codes, so handlers never leak SQL or internal detail to a client.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		writeJSON(w, http.StatusBadRequest, errorResponse{errorBody{
			Code: CodeValidation, Message: err.Error(),
		}})
	case errors.Is(err, domain.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errorResponse{errorBody{
			Code: CodeUnauthorized, Message: "authentication required",
		}})
	case errors.Is(err, domain.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorResponse{errorBody{
			Code: CodeForbidden, Message: "access denied",
		}})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{errorBody{
			Code: CodeNotFound, Message: "resource not found",
		}})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{errorBody{
			Code: CodeConflict, Message: err.Error(),
		}})
	default:
		// The detail is logged, not returned.
		slog.Error("unhandled error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{errorBody{
			Code: CodeInternal, Message: "internal server error",
		}})
	}
}

// decodeJSON reads a JSON request body with a size limit.
//
// It rejects unknown fields so a typo in a client request is reported
// rather than silently ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return err
	}
	return nil
}

// parseUUIDParam reads a UUID path parameter.
func parseUUIDParam(r *http.Request, name string) (string, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return "", errors.New("missing parameter: " + name)
	}
	return v, nil
}
