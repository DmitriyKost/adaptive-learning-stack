package httptransport

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"auth-service/internal/domain"
)

type errorResponse struct {
	Error string `json:"error"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type authResponse struct {
	AccessToken           string       `json:"access_token"`
	RefreshToken          string       `json:"refresh_token"`
	TokenType             string       `json:"token_type"`
	AccessTokenExpiresAt  time.Time    `json:"access_token_expires_at"`
	RefreshTokenExpiresAt time.Time    `json:"refresh_token_expires_at"`
	User                  userResponse `json:"user"`
}

func toUserResponse(user domain.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func toAuthResponse(result domain.AuthResult) authResponse {
	return authResponse{
		AccessToken:           result.Tokens.AccessToken,
		RefreshToken:          result.Tokens.RefreshToken,
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  result.Tokens.AccessTokenExpiresAt,
		RefreshTokenExpiresAt: result.Tokens.RefreshTokenExpiresAt,
		User:                  toUserResponse(result.User),
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal_error"

	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		status = http.StatusBadRequest
		message = "invalid_input"
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		message = "user_already_exists"
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
		message = "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
		message = "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		message = "not_found"
	}

	writeJSON(w, status, errorResponse{Error: message})
}
