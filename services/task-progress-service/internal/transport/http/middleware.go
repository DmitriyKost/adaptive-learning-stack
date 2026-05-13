package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"task-progress-service/internal/domain"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		requestID := requestID()
		w.Header().Set("X-Request-ID", requestID)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("http request", slog.String("request_id", requestID), slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Int("status", rec.status), slog.Duration("duration", time.Since(startedAt)))
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("panic recovered", "panic", recovered)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal_error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.allowGatewayHeaders {
			userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
			role := strings.TrimSpace(r.Header.Get("X-User-Role"))
			if userID != "" {
				if role == "" {
					role = "student"
				}
				next.ServeHTTP(w, r.WithContext(contextWithClaims(r.Context(), domain.AccessClaims{UserID: userID, Role: role, TokenType: "access"})))
				return
			}
		}

		header := r.Header.Get("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		claims, err := s.tokens.ValidateAccessToken(strings.TrimSpace(parts[1]))
		if err != nil {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(contextWithClaims(r.Context(), claims)))
	})
}

func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := claimsFromContext(r.Context())
		if !ok {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		if claims.Role != "admin" {
			writeError(w, domain.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
