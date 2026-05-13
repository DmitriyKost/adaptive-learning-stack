package httptransport

import (
	"encoding/json"
	"io"
	"net/http"

	"auth-service/internal/domain"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}

	result, err := s.auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		s.log.Warn("register failed", "error", err)
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAuthResponse(result))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}

	result, err := s.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		s.log.Warn("login failed", "error", err)
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toAuthResponse(result))
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}

	result, err := s.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		s.log.Warn("refresh failed", "error", err)
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toAuthResponse(result))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}

	if err := s.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		s.log.Warn("logout failed", "error", err)
		writeError(w, err)
		return
	}

	writeNoContent(w)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	user, err := s.auth.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (s *Server) userByID(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	userID := r.PathValue("id")
	if userID == "" {
		writeError(w, domain.ErrInvalidInput)
		return
	}

	if claims.UserID != userID && claims.Role != domain.RoleAdmin {
		writeError(w, domain.ErrForbidden)
		return
	}

	user, err := s.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return domain.ErrInvalidInput
	}

	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return domain.ErrInvalidInput
	} else if err != io.EOF {
		return domain.ErrInvalidInput
	}

	return nil
}
