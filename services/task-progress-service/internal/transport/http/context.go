package httptransport

import (
	"context"

	"task-progress-service/internal/domain"
)

type claimsKey struct{}

func contextWithClaims(ctx context.Context, claims domain.AccessClaims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

func claimsFromContext(ctx context.Context) (domain.AccessClaims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(domain.AccessClaims)
	return claims, ok
}
