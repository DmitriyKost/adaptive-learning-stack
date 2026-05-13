package httptransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"api-gateway/internal/config"
	"api-gateway/internal/domain"
	"api-gateway/internal/service"
)

type Server struct {
	cfg                 config.Config
	tokens              *service.TokenValidator
	log                 *slog.Logger
	requestTimeout      time.Duration
	corsAllowedOrigins  []string
	corsAllowedHeaders  string
	corsAllowedMethods  string
	auth                *backendProxy
	taskProgress        *backendProxy
	playground          *backendProxy
	analytics           *backendProxy
	readinessHTTPClient *http.Client
}

func NewServer(cfg config.Config, log *slog.Logger) (*Server, error) {
	authProxy, err := newBackendProxy("auth-service", cfg.AuthServiceURL, log)
	if err != nil {
		return nil, err
	}
	taskProxy, err := newBackendProxy("task-progress-service", cfg.TaskProgressServiceURL, log)
	if err != nil {
		return nil, err
	}
	playgroundProxy, err := newBackendProxy("playground-service", cfg.PlaygroundServiceURL, log)
	if err != nil {
		return nil, err
	}
	analyticsProxy, err := newBackendProxy("analytics-service", cfg.AnalyticsServiceURL, log)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:                 cfg,
		tokens:              service.NewTokenValidator(cfg.JWTSecret),
		log:                 log,
		requestTimeout:      cfg.RequestTimeout,
		corsAllowedOrigins:  cfg.CORSAllowedOrigins,
		corsAllowedHeaders:  cfg.CORSAllowedHeaders,
		corsAllowedMethods:  cfg.CORSAllowedMethods,
		auth:                authProxy,
		taskProgress:        taskProxy,
		playground:          playgroundProxy,
		analytics:           analyticsProxy,
		readinessHTTPClient: &http.Client{Timeout: 2 * time.Second},
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /ready", s.ready)

	mux.Handle("/auth/", s.publicProxy(s.auth))

	mux.Handle("/users/me", s.protectedProxy(s.auth))
	mux.Handle("/users/", s.protectedProxy(s.auth))

	mux.Handle("/tasks", s.protectedProxy(s.taskProgress))
	mux.Handle("/tasks/", s.protectedProxy(s.taskProgress))
	mux.Handle("/progress/me", s.protectedProxy(s.taskProgress))
	mux.Handle("/progress/skills", s.protectedProxy(s.taskProgress))
	mux.Handle("/users/me/learning-graph", s.protectedProxy(s.taskProgress))
	mux.Handle("/graphs", s.protectedProxy(s.taskProgress))
	mux.Handle("/graphs/", s.protectedProxy(s.taskProgress))
	mux.Handle("/skills", s.protectedProxy(s.taskProgress))

	mux.Handle("/execute", s.protectedProxy(s.playground))
	mux.Handle("/playground/", s.protectedProxy(s.playground))

	// Analytics has no public user-facing API yet. Internal endpoints such as
	// /internal/hints/generate are intentionally not exposed through the gateway.
	mux.HandleFunc("/analytics/", s.analyticsNotExposed)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, domain.ErrNotFound)
	})

	var handler http.Handler = mux
	handler = s.recoveryMiddleware(handler)
	handler = s.loggingMiddleware(handler)
	handler = s.corsMiddleware(handler)
	handler = s.timeoutMiddleware(handler)
	return handler
}

func (s *Server) publicProxy(proxy *backendProxy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.cleanGatewayIdentityHeaders(r)
		proxy.ServeHTTP(w, r)
	})
}

func (s *Server) protectedProxy(proxy *backendProxy) http.Handler {
	return s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isAdminRoute(r) {
			claims, ok := claimsFromContext(r.Context())
			if !ok {
				writeError(w, domain.ErrUnauthorized)
				return
			}
			if claims.Role != domain.RoleAdmin {
				writeError(w, domain.ErrForbidden)
				return
			}
		}

		claims, _ := claimsFromContext(r.Context())
		s.prepareUpstreamHeaders(r, claims)
		proxy.ServeHTTP(w, r)
	}))
}

func (s *Server) cleanGatewayIdentityHeaders(r *http.Request) {
	r.Header.Del("X-User-ID")
	r.Header.Del("X-User-Role")
}

func (s *Server) prepareUpstreamHeaders(r *http.Request, claims domain.AccessClaims) {
	s.cleanGatewayIdentityHeaders(r)
	r.Header.Set("X-User-ID", claims.UserID)
	r.Header.Set("X-User-Role", claims.Role)
	if !s.cfg.ForwardAuthorization {
		r.Header.Del("Authorization")
	}
}

func (s *Server) isAdminRoute(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		return false
	}
	path := strings.Trim(r.URL.Path, "/")
	if path == "skills" || path == "graphs" {
		return true
	}
	if strings.HasPrefix(path, "graphs/") && (strings.HasSuffix(path, "/skills") || strings.HasSuffix(path, "/dependencies")) {
		return true
	}
	return false
}

func (s *Server) analyticsNotExposed(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_exposed"})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type readinessResponse struct {
	Status   string                      `json:"status"`
	Services map[string]serviceReadiness `json:"services"`
}

type serviceReadiness struct {
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	services := map[string]string{
		"auth-service":          s.cfg.AuthServiceURL + "/health",
		"task-progress-service": s.cfg.TaskProgressServiceURL + "/ready",
		"playground-service":    s.cfg.PlaygroundServiceURL + "/ready",
		"analytics-service":     s.cfg.AnalyticsServiceURL + "/ready",
	}

	result := readinessResponse{Status: "ready", Services: make(map[string]serviceReadiness, len(services))}
	if !s.cfg.CheckDownstreamReady {
		for name := range services {
			result.Services[name] = serviceReadiness{Status: "skipped"}
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	for name, url := range services {
		status := s.checkService(ctx, url)
		result.Services[name] = status
		if status.Status != "ready" {
			result.Status = "degraded"
		}
	}

	if result.Status == "ready" {
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, result)
}

func (s *Server) checkService(ctx context.Context, rawURL string) serviceReadiness {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return serviceReadiness{Status: "unavailable", Error: err.Error()}
	}
	resp, err := s.readinessHTTPClient.Do(req)
	if err != nil {
		return serviceReadiness{Status: "unavailable", Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return serviceReadiness{Status: "ready", StatusCode: resp.StatusCode}
	}
	return serviceReadiness{Status: "unavailable", StatusCode: resp.StatusCode}
}

func debugJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
