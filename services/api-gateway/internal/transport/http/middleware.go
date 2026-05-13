package httptransport

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"api-gateway/internal/domain"
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
		reqID := r.Header.Get("X-Request-ID")
		if strings.TrimSpace(reqID) == "" {
			reqID = requestID()
		}
		w.Header().Set("X-Request-ID", reqID)
		r.Header.Set("X-Request-ID", reqID)

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		userID := ""
		if claims, ok := claimsFromContext(r.Context()); ok {
			userID = claims.UserID
		}
		s.log.Info("http request",
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", recorder.status),
			slog.Duration("duration", time.Since(startedAt)),
			slog.String("user_id", userID),
		)
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

func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	return http.TimeoutHandler(next, s.requestTimeout, `{"error":"gateway_timeout"}`)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.isOriginAllowed(origin) {
			if len(s.corsAllowedOrigins) == 1 && s.corsAllowedOrigins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", s.corsAllowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", s.corsAllowedHeaders)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID,Retry-After")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) isOriginAllowed(origin string) bool {
	for _, allowed := range s.corsAllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := claimsFromContext(r.Context())
		if !ok {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		if claims.Role != domain.RoleAdmin {
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
