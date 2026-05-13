package httptransport

import (
	"context"

	"auth-service/internal/service"
)

type contextKey string

const claimsContextKey contextKey = "auth_claims"

func contextWithClaims(ctx context.Context, claims service.AccessClaims) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

func claimsFromContext(ctx context.Context) (service.AccessClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(service.AccessClaims)
	return claims, ok
}
