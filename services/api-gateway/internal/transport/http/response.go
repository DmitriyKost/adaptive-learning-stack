package httptransport

import (
	"encoding/json"
	"errors"
	"net/http"

	"api-gateway/internal/domain"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
	case errors.Is(err, domain.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "forbidden"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
	case errors.Is(err, domain.ErrBadGateway):
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "bad_gateway"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal_error"})
	}
}
