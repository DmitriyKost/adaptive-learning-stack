package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"playground-service/internal/domain"
)

type TaskReference struct {
	ReferenceSQL   string
	OrderSensitive bool
}

type TaskReferenceProvider interface {
	Reference(ctx context.Context, taskID string) (TaskReference, error)
}

type HTTPTaskReferenceClient struct {
	baseURL string
	client  *http.Client
}

type taskComparisonPolicyResponse struct {
	OrderSensitive bool `json:"order_sensitive"`
}

type taskReferenceResponse struct {
	TaskID           string                       `json:"task_id"`
	ReferenceSQL     string                       `json:"reference_sql"`
	ComparisonPolicy taskComparisonPolicyResponse `json:"comparison_policy"`
}

func NewHTTPTaskReferenceClient(baseURL string, timeout time.Duration) (*HTTPTaskReferenceClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, domain.ErrInvalidInput
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid task-progress base url %q", baseURL)
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &HTTPTaskReferenceClient{baseURL: baseURL, client: &http.Client{Timeout: timeout}}, nil
}

func (c *HTTPTaskReferenceClient) Reference(ctx context.Context, taskID string) (TaskReference, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskReference{}, domain.ErrInvalidInput
	}
	endpoint := c.baseURL + "/internal/tasks/" + url.PathEscape(taskID) + "/reference"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TaskReference{}, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return TaskReference{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var payload taskReferenceResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return TaskReference{}, err
		}
		referenceSQL := strings.TrimSpace(payload.ReferenceSQL)
		if referenceSQL == "" {
			return TaskReference{}, domain.ErrNotFound
		}
		return TaskReference{
			ReferenceSQL:   referenceSQL,
			OrderSensitive: payload.ComparisonPolicy.OrderSensitive,
		}, nil
	case http.StatusNotFound:
		return TaskReference{}, domain.ErrNotFound
	case http.StatusBadRequest:
		return TaskReference{}, domain.ErrInvalidInput
	default:
		return TaskReference{}, fmt.Errorf("task-progress reference endpoint returned %d", resp.StatusCode)
	}
}
