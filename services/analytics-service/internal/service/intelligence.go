package service

import (
	"context"

	"analytics-service/internal/domain"
)

type IntelligenceClient interface {
	Ready(ctx context.Context) error
	EvaluateTaskCompletion(ctx context.Context, input domain.LLMContext) (domain.IntelligenceResult, error)
	GenerateHint(ctx context.Context, input domain.HintContextForLLM) (domain.HintGenerateResponse, string, string, error)
}
