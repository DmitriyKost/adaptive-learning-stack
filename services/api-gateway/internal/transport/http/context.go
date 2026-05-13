package httptransport

import (
	"context"

	"api-gateway/internal/domain"
)

type contextKey string

const claimsContextKey contextKey = "claims"

func contextWithClaims(ctx context.Context, claims domain.AccessClaims) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

func claimsFromContext(ctx context.Context) (domain.AccessClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(domain.AccessClaims)
	return claims, ok
}
