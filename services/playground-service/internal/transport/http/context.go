package httptransport

import (
	"context"

	"playground-service/internal/domain"
)

type claimsContextKey struct{}

func contextWithClaims(ctx context.Context, claims domain.AccessClaims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

func claimsFromContext(ctx context.Context) (domain.AccessClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(domain.AccessClaims)
	return claims, ok
}
