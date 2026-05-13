package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"task-progress-service/internal/domain"
)

type HintGeneratorClient interface {
	GenerateHint(ctx context.Context, req domain.HintGenerateRequest) (domain.HintGenerateResponse, error)
}

type HTTPHintGeneratorClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPHintGeneratorClient(baseURL string, timeout time.Duration) *HTTPHintGeneratorClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &HTTPHintGeneratorClient{baseURL: baseURL, client: &http.Client{Timeout: timeout}}
}

func (c *HTTPHintGeneratorClient) GenerateHint(ctx context.Context, req domain.HintGenerateRequest) (domain.HintGenerateResponse, error) {
	if c == nil || c.baseURL == "" {
		return domain.HintGenerateResponse{}, domain.ErrHintGenerationFailed
	}
	body, err := json.Marshal(req)
	if err != nil {
		return domain.HintGenerateResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/hints/generate", bytes.NewReader(body))
	if err != nil {
		return domain.HintGenerateResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return domain.HintGenerateResponse{}, domain.ErrHintGenerationFailed
		}
		return domain.HintGenerateResponse{}, fmt.Errorf("generate hint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.HintGenerateResponse{}, domain.ErrHintGenerationFailed
	}
	var out domain.HintGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return domain.HintGenerateResponse{}, err
	}
	if out.HintID == "" || out.Message == "" {
		return domain.HintGenerateResponse{}, domain.ErrHintGenerationFailed
	}
	return out, nil
}
