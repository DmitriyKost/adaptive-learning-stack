package httptransport

import (
	"log/slog"
	"net/http"

	"auth-service/internal/service"
)

type Server struct {
	auth   *service.AuthUsecase
	tokens *service.TokenService
	log    *slog.Logger
}

func NewRouter(auth *service.AuthUsecase, tokens *service.TokenService, log *slog.Logger) http.Handler {
	server := &Server{auth: auth, tokens: tokens, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)

	mux.HandleFunc("POST /auth/register", server.register)
	mux.HandleFunc("POST /auth/login", server.login)
	mux.HandleFunc("POST /auth/refresh", server.refresh)
	mux.HandleFunc("POST /auth/logout", server.logout)

	mux.Handle("GET /users/me", server.authMiddleware(http.HandlerFunc(server.me)))
	mux.Handle("GET /users/{id}", server.authMiddleware(http.HandlerFunc(server.userByID)))

	var handler http.Handler = mux
	handler = server.recoveryMiddleware(handler)
	handler = server.loggingMiddleware(handler)

	return handler
}
