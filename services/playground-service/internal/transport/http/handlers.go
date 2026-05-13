package httptransport

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"playground-service/internal/domain"
)

type workspaceResponse struct {
	UserID     string `json:"user_id"`
	SchemaName string `json:"schema_name"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) execute(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	var req domain.ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domain.ErrInvalidInput)
		return
	}

	response, err := s.playground.Execute(r.Context(), claims.UserID, req)
	if err != nil {
		s.log.Error("execute failed", slog.String("user_id", claims.UserID), slog.String("task_id", req.TaskID), slog.Any("error", err))
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	workspace, err := s.playground.EnsureWorkspace(r.Context(), claims.UserID)
	if err != nil {
		s.log.Error("ensure workspace failed", slog.String("user_id", claims.UserID), slog.Any("error", err))
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, workspaceResponse{UserID: claims.UserID, SchemaName: workspace.SchemaName})
}

func (s *Server) resetWorkspace(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	workspace, err := s.playground.ResetWorkspace(r.Context(), claims.UserID)
	if err != nil {
		s.log.Error("reset workspace failed", slog.String("user_id", claims.UserID), slog.Any("error", err))
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, workspaceResponse{UserID: claims.UserID, SchemaName: workspace.SchemaName})
}
