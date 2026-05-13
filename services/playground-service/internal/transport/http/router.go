package httptransport

import (
	"log/slog"
	"net/http"

	"playground-service/internal/service"
)

type Server struct {
	playground            *service.PlaygroundUsecase
	tokens                *service.TokenService
	trustedGatewayHeaders bool
	log                   *slog.Logger
}

func NewRouter(playground *service.PlaygroundUsecase, tokens *service.TokenService, trustedGatewayHeaders bool, log *slog.Logger) http.Handler {
	server := &Server{
		playground:            playground,
		tokens:                tokens,
		trustedGatewayHeaders: trustedGatewayHeaders,
		log:                   log,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /ready", server.health)
	mux.Handle("POST /execute", server.authMiddleware(http.HandlerFunc(server.execute)))
	mux.Handle("POST /playground/execute", server.authMiddleware(http.HandlerFunc(server.execute)))
	mux.Handle("GET /playground/workspace", server.authMiddleware(http.HandlerFunc(server.workspace)))
	mux.Handle("POST /playground/reset", server.authMiddleware(http.HandlerFunc(server.resetWorkspace)))

	var handler http.Handler = mux
	handler = server.recoveryMiddleware(handler)
	handler = server.loggingMiddleware(handler)

	return handler
}
