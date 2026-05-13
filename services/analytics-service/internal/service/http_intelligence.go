package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"analytics-service/internal/config"
	"analytics-service/internal/domain"
)

// HTTPIntelligenceClient calls the external Python intelligence service over HTTP.
type HTTPIntelligenceClient struct {
	cfg    config.Config
	client *http.Client
}

func NewHTTPIntelligenceClient(cfg config.Config) *HTTPIntelligenceClient {
	return &HTTPIntelligenceClient{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.IntelligenceHTTPTimeout,
		},
	}
}

func (c *HTTPIntelligenceClient) base() string {
	return strings.TrimRight(strings.TrimSpace(c.cfg.IntelligenceBaseURL), "/")
}

func (c *HTTPIntelligenceClient) setAuth(req *http.Request) {
	key := strings.TrimSpace(c.cfg.IntelligenceAPIKey)
	if key == "" {
		return
	}
	req.Header.Set("X-API-Key", key)
}

func (c *HTTPIntelligenceClient) Ready(ctx context.Context) error {
	url := c.base() + c.cfg.IntelligenceReadyPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("intelligence ready: HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}
	return nil
}

func structToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func attemptToPythonMap(a domain.AttemptLog) map[string]any {
	m := map[string]any{
		"attempt_number":    a.AttemptNumber,
		"submitted_sql":     a.SubmittedSQL,
		"is_correct":        a.IsCorrect,
		"execution_time_ms": a.ExecutionTimeMS,
		"hint_used":         a.HintUsed,
		"hint_requested":    a.HintRequested,
		"error_type":        a.ErrorType,
		"error_message":     a.ErrorMessage,
		"execution_success": a.ExecutionSuccess,
		"reference_sql":     a.ReferenceSQL,
		"row_count":         a.RowCount,
		"hints_count":       a.HintsCount,
		"hint_type":         a.HintType,
	}
	return m
}

func attemptsPythonSlice(attempts []domain.AttemptLog) []any {
	out := make([]any, 0, len(attempts))
	for _, a := range attempts {
		out = append(out, attemptToPythonMap(a))
	}
	return out
}

func (c *HTTPIntelligenceClient) buildEvaluateBody(in domain.LLMContext) ([]byte, error) {
	taskMap, err := structToMap(in.Task)
	if err != nil {
		return nil, fmt.Errorf("task: %w", err)
	}
	completedMap, err := structToMap(in.CompletedEvent)
	if err != nil {
		return nil, fmt.Errorf("completed_event: %w", err)
	}
	var graphMap map[string]any
	if in.GraphState != nil {
		graphMap, err = structToMap(*in.GraphState)
		if err != nil {
			return nil, fmt.Errorf("graph_state: %w", err)
		}
	} else {
		graphMap = map[string]any{}
	}
	body := map[string]any{
		"user_id":                       in.UserID,
		"task_id":                       in.TaskID,
		"attempt_id":                    in.AttemptID,
		"task":                          taskMap,
		"attempts":                      attemptsPythonSlice(in.Attempts),
		"completed_event":               completedMap,
		"graph_state":                   graphMap,
		"recent_recommendations":        []any{},
		"recent_model_updates":          []any{},
		"prepared_at":                   in.PreparedAt.UTC().Format(time.RFC3339Nano),
		"include_career_recommendation": c.cfg.IntelligenceIncludeCareer,
		"assessment_output":             "absolute",
		"assessment_apply_mode":         "merge",
	}
	return json.Marshal(body)
}

func (c *HTTPIntelligenceClient) buildHintBody(in domain.HintContextForLLM) ([]byte, error) {
	taskMap, err := structToMap(in.Task)
	if err != nil {
		return nil, fmt.Errorf("task: %w", err)
	}
	prevJSON, err := json.Marshal(in.PreviousHints)
	if err != nil {
		return nil, err
	}
	var prevList []any
	if len(prevJSON) > 0 && string(prevJSON) != "null" {
		if err := json.Unmarshal(prevJSON, &prevList); err != nil {
			return nil, err
		}
	}
	body := map[string]any{
		"user_id":                in.UserID,
		"task_id":                in.TaskID,
		"current_attempt_number": in.CurrentAttemptNumber,
		"task":                   taskMap,
		"attempts":               attemptsPythonSlice(in.Attempts),
		"previous_hints":         prevList,
		"prepared_at":            in.PreparedAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(body)
}

func (c *HTTPIntelligenceClient) EvaluateTaskCompletion(ctx context.Context, input domain.LLMContext) (domain.IntelligenceResult, error) {
	reqBytes, err := c.buildEvaluateBody(input)
	if err != nil {
		return domain.IntelligenceResult{}, err
	}
	url := c.base() + c.cfg.IntelligenceEvaluatePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return domain.IntelligenceResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return domain.IntelligenceResult{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.IntelligenceResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.IntelligenceResult{}, fmt.Errorf("intelligence evaluate: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 800))
	}
	var outer struct {
		Skill  json.RawMessage `json:"skill_assessment"`
		Career json.RawMessage `json:"career_recommendation"`
	}
	if err := json.Unmarshal(respBody, &outer); err != nil {
		return domain.IntelligenceResult{}, fmt.Errorf("decode envelope: %w", err)
	}
	var assessment domain.AnalyticsSkillAssessmentUpdatedEvent
	if err := json.Unmarshal(outer.Skill, &assessment); err != nil {
		return domain.IntelligenceResult{}, fmt.Errorf("decode skill_assessment: %w", err)
	}
	var career *domain.CareerRecommendationCreatedEvent
	if len(outer.Career) > 0 && string(outer.Career) != "null" {
		var careerEvent domain.CareerRecommendationCreatedEvent
		if err := json.Unmarshal(outer.Career, &careerEvent); err == nil && careerEvent.EventID != "" {
			career = &careerEvent
		}
	}
	return domain.IntelligenceResult{
		Assessment:         assessment,
		Career:             career,
		RequestContextJSON: string(reqBytes),
		ResponseJSON:       string(respBody),
	}, nil
}

func (c *HTTPIntelligenceClient) GenerateHint(ctx context.Context, input domain.HintContextForLLM) (domain.HintGenerateResponse, string, string, error) {
	reqBytes, err := c.buildHintBody(input)
	if err != nil {
		return domain.HintGenerateResponse{}, string(reqBytes), "", err
	}
	url := c.base() + c.cfg.IntelligenceHintPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return domain.HintGenerateResponse{}, string(reqBytes), "", err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return domain.HintGenerateResponse{}, string(reqBytes), "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.HintGenerateResponse{}, string(reqBytes), "", err
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return domain.HintGenerateResponse{}, string(reqBytes), string(respBody), fmt.Errorf("hint: service unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.HintGenerateResponse{}, string(reqBytes), string(respBody), fmt.Errorf("hint: HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}
	var out domain.HintGenerateResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return domain.HintGenerateResponse{}, string(reqBytes), string(respBody), fmt.Errorf("decode hint response: %w", err)
	}
	return out, string(reqBytes), string(respBody), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
