package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"analytics-service/internal/config"
	"analytics-service/internal/domain"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Repository struct {
	conn driver.Conn
	log  *slog.Logger
}

func Connect(ctx context.Context, cfg config.Config, log *slog.Logger) (*Repository, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:        []string{cfg.ClickHouseAddr},
		Auth:        clickhouse.Auth{Database: cfg.ClickHouseDatabase, Username: cfg.ClickHouseUsername, Password: cfg.ClickHousePassword},
		DialTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second,
		Settings:    clickhouse.Settings{"max_execution_time": 60},
		Protocol:    clickhouse.Native,
		TLS:         nil,
	})
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}
	return &Repository{conn: conn, log: log}, nil
}

func (r *Repository) Close() error                   { return r.conn.Close() }
func (r *Repository) Ping(ctx context.Context) error { return r.conn.Ping(ctx) }

func (r *Repository) AutoMigrate(ctx context.Context) error {
	for _, stmt := range migrationStatements {
		if err := r.conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("clickhouse migration failed: %w", err)
		}
	}
	return nil
}

func (r *Repository) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	if eventID == "" {
		return false, nil
	}
	var count uint64
	err := r.conn.QueryRow(ctx, `SELECT count() FROM raw_events WHERE event_id = ?`, eventID).Scan(&count)
	return count > 0, err
}

func (r *Repository) LogRawEvent(ctx context.Context, e domain.RawEventLog) error {
	if e.EventTime.IsZero() {
		e.EventTime = time.Now().UTC()
	}
	return r.conn.Exec(ctx, `
		INSERT INTO raw_events
		(event_id, event_type, user_id, task_id, attempt_id, source_topic, kafka_partition, kafka_offset, event_time, ingested_at, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3), ?)`,
		e.EventID, e.EventType, e.UserID, e.TaskID, e.AttemptID, e.SourceTopic, int32(e.KafkaPartition), e.KafkaOffset, e.EventTime, e.Payload)
}

func (r *Repository) StoreTaskAttempt(ctx context.Context, e domain.TaskCheckedEvent) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	taskJSON := mustJSON(e.Task)
	execJSON := mustJSON(e.Execution)
	actualJSON := mustJSON(e.ActualResult)
	expectedJSON := mustJSON(e.ExpectedResult)
	skills := make([]string, 0, len(e.Task.Skills)+len(e.AffectedSkills))
	seen := map[string]struct{}{}
	for _, s := range e.Task.Skills {
		code := s.SkillCode
		if code == "" {
			code = s.SkillID
		}
		if code != "" {
			if _, ok := seen[code]; !ok {
				skills = append(skills, code)
				seen[code] = struct{}{}
			}
		}
	}
	for _, s := range e.AffectedSkills {
		code := s.SkillCode
		if code == "" {
			code = s.SkillID
		}
		if code != "" {
			if _, ok := seen[code]; !ok {
				skills = append(skills, code)
				seen[code] = struct{}{}
			}
		}
	}
	return r.conn.Exec(ctx, `
		INSERT INTO task_attempt_logs
		(event_id, user_id, task_id, attempt_id, attempt_number, execution_success, is_correct, submitted_sql, reference_sql,
		 error_type, error_message, execution_time_ms, row_count, hint_requested, hint_used, hint_type, hints_count,
		 skills, task_json, execution_json, actual_result_json, expected_result_json, created_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		e.EventID, e.UserID, e.TaskID, e.AttemptID, int32(e.AttemptNumber), e.ExecutionSuccess, e.IsCorrect, e.SubmittedSQL, e.ReferenceSQL,
		e.ErrorType, e.ErrorMessage, e.ExecutionTimeMS, int32(e.RowCount), e.Hint.HintRequested, e.Hint.HintUsed, e.Hint.HintType, int32(e.Hint.HintsCount),
		skills, taskJSON, execJSON, actualJSON, expectedJSON, created)
}

func (r *Repository) StoreTaskCompletion(ctx context.Context, e domain.TaskCompletedEvent) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	graphCode := ""
	track := ""
	if e.GraphState != nil {
		graphCode = e.GraphState.Graph.Code
		track = e.GraphState.Profile.ProfessionalTrack
	}
	return r.conn.Exec(ctx, `
		INSERT INTO task_completion_logs
		(event_id, user_id, task_id, attempt_id, graph_code, professional_track, graph_state_json, task_json, execution_json, hint_json, created_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		e.EventID, e.UserID, e.TaskID, e.AttemptID, graphCode, track, mustJSON(e.GraphState), mustJSON(e.Task), mustJSON(e.Execution), mustJSON(e.Hint), created)
}

func (r *Repository) StoreTaskRecommendation(ctx context.Context, e domain.TaskRecommendedEvent, payload string) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return r.conn.Exec(ctx, `
		INSERT INTO task_recommendation_logs
		(event_id, user_id, task_id, graph_code, score, reason, repeat_mode, candidate_source, payload, created_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		e.EventID, e.UserID, e.TaskID, e.GraphCode, e.Score, e.Reason, e.RepeatMode, e.CandidateSource, payload, created)
}

func (r *Repository) StoreLearnerModelUpdate(ctx context.Context, e domain.LearnerModelUpdatedEvent) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	analysisRunID := ""
	if e.Analysis != nil {
		analysisRunID = e.Analysis.AnalysisRunID
	}
	return r.conn.Exec(ctx, `
		INSERT INTO learner_model_update_logs
		(event_id, user_id, skill_id, skill_code, old_mastery_score, new_mastery_score, old_confidence, new_confidence, source, reason, analysis_run_id, created_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		e.EventID, e.UserID, e.SkillID, e.SkillCode, e.OldMasteryScore, e.NewMasteryScore, e.OldConfidence, e.NewConfidence, e.Source, e.Reason, analysisRunID, created)
}

func (r *Repository) LoadLLMContext(ctx context.Context, completed domain.TaskCompletedEvent) (domain.LLMContext, error) {
	attempts, err := r.loadAttempts(ctx, completed.UserID, completed.TaskID)
	if err != nil {
		return domain.LLMContext{}, err
	}
	recs, _ := r.loadRecentRecommendations(ctx, completed.UserID, 10)
	updates, _ := r.loadRecentModelUpdates(ctx, completed.UserID, 50)
	return domain.LLMContext{
		UserID:                completed.UserID,
		TaskID:                completed.TaskID,
		AttemptID:             completed.AttemptID,
		Task:                  completed.Task,
		Attempts:              attempts,
		CompletedEvent:        completed,
		GraphState:            completed.GraphState,
		RecentRecommendations: recs,
		RecentModelUpdates:    updates,
		PreparedAt:            time.Now().UTC(),
	}, nil
}

func (r *Repository) loadAttempts(ctx context.Context, userID, taskID string) ([]domain.AttemptLog, error) {
	rows, err := r.conn.Query(ctx, `
		SELECT event_id, user_id, task_id, attempt_id, attempt_number, execution_success, is_correct, submitted_sql, reference_sql,
		       error_type, error_message, execution_time_ms, row_count, hint_requested, hint_used, hint_type, hints_count,
		       skills, task_json, execution_json, actual_result_json, expected_result_json, created_at
		FROM task_attempt_logs
		WHERE user_id = ? AND task_id = ?
		ORDER BY created_at ASC, attempt_number ASC`, userID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AttemptLog{}
	for rows.Next() {
		var a domain.AttemptLog
		var attemptNumber, rowCount, hintsCount int32
		if err := rows.Scan(&a.EventID, &a.UserID, &a.TaskID, &a.AttemptID, &attemptNumber, &a.ExecutionSuccess, &a.IsCorrect,
			&a.SubmittedSQL, &a.ReferenceSQL, &a.ErrorType, &a.ErrorMessage, &a.ExecutionTimeMS, &rowCount, &a.HintRequested,
			&a.HintUsed, &a.HintType, &hintsCount, &a.Skills, &a.TaskJSON, &a.ExecutionJSON, &a.ActualResultJSON,
			&a.ExpectedResultJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.AttemptNumber = int(attemptNumber)
		a.RowCount = int(rowCount)
		a.HintsCount = int(hintsCount)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repository) loadRecentRecommendations(ctx context.Context, userID string, limit int) ([]domain.RecommendationLog, error) {
	rows, err := r.conn.Query(ctx, `
		SELECT event_id, user_id, task_id, graph_code, score, reason, repeat_mode, candidate_source, payload, created_at
		FROM task_recommendation_logs
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, userID, uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RecommendationLog
	for rows.Next() {
		var rlog domain.RecommendationLog
		if err := rows.Scan(&rlog.EventID, &rlog.UserID, &rlog.TaskID, &rlog.GraphCode, &rlog.Score, &rlog.Reason, &rlog.RepeatMode, &rlog.CandidateSource, &rlog.Payload, &rlog.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rlog)
	}
	return out, rows.Err()
}

func (r *Repository) loadRecentModelUpdates(ctx context.Context, userID string, limit int) ([]domain.LearnerModelUpdateLog, error) {
	rows, err := r.conn.Query(ctx, `
		SELECT event_id, user_id, skill_id, skill_code, old_mastery_score, new_mastery_score, old_confidence, new_confidence, source, reason, analysis_run_id, created_at
		FROM learner_model_update_logs
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?`, userID, uint64(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LearnerModelUpdateLog
	for rows.Next() {
		var u domain.LearnerModelUpdateLog
		if err := rows.Scan(&u.EventID, &u.UserID, &u.SkillID, &u.SkillCode, &u.OldMasteryScore, &u.NewMasteryScore, &u.OldConfidence, &u.NewConfidence, &u.Source, &u.Reason, &u.AnalysisRunID, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) SaveAnalysisRun(ctx context.Context, run domain.AnalysisRunLog) error {
	return r.conn.Exec(ctx, `
		INSERT INTO llm_analysis_runs
		(run_id, user_id, source_task_id, source_attempt_id, status, model_version, prompt_version, request_context, response_payload, error_message, started_at, completed_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		run.RunID, run.UserID, run.SourceTaskID, run.SourceAttemptID, run.Status, run.ModelVersion, run.PromptVersion, run.RequestContext, run.ResponsePayload, run.ErrorMessage, run.StartedAt, run.CompletedAt)
}

func (r *Repository) SaveSkillAssessments(ctx context.Context, runID, userID string, items []domain.SkillAssessment, createdAt time.Time) error {
	for _, item := range items {
		var mastery, conf, corr, indep, eff float64
		if item.MasteryScore != nil {
			mastery = *item.MasteryScore
		}
		if item.Confidence != nil {
			conf = *item.Confidence
		}
		if item.Components != nil {
			if item.Components.Correctness != nil {
				corr = *item.Components.Correctness
			}
			if item.Components.Independence != nil {
				indep = *item.Components.Independence
			}
			if item.Components.Efficiency != nil {
				eff = *item.Components.Efficiency
			}
		}
		if err := r.conn.Exec(ctx, `
			INSERT INTO skill_assessment_logs
			(analysis_run_id, user_id, skill_id, skill_code, mastery_score, confidence, correctness, independence, efficiency, reason, created_at, ingested_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
			runID, userID, item.SkillID, item.SkillCode, mastery, conf, corr, indep, eff, item.Reason, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SaveSkillRecommendations(ctx context.Context, runID, userID string, items []domain.RecommendedSkill, createdAt time.Time) error {
	for _, item := range items {
		if err := r.conn.Exec(ctx, `
			INSERT INTO skill_recommendation_logs
			(analysis_run_id, user_id, skill_id, skill_code, priority, recommended_action, reason, created_at, ingested_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
			runID, userID, item.SkillID, item.SkillCode, item.Priority, item.RecommendedAction, item.Reason, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) SaveCareerRecommendation(ctx context.Context, e domain.CareerRecommendationCreatedEvent) error {
	created := e.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return r.conn.Exec(ctx, `
		INSERT INTO career_recommendation_logs
		(event_id, user_id, analysis_run_id, primary_track, primary_title, score, recommended_graph_code, strong_skills, weak_skills, explanation, payload, created_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		e.EventID, e.UserID, metaRunID(e.Analysis), e.PrimaryTrack, e.PrimaryTitle, e.Score, e.RecommendedGraphCode, e.StrongSkills, e.WeakSkills, e.Explanation, mustJSON(e), created)
}

func metaRunID(meta *domain.AnalysisMetadata) string {
	if meta == nil {
		return ""
	}
	return meta.AnalysisRunID
}

func mustJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (r *Repository) LoadHintContext(ctx context.Context, req domain.HintGenerateRequest) (domain.HintContextForLLM, error) {
	attempts, err := r.loadAttempts(ctx, req.UserID, req.TaskID)
	if err != nil {
		return domain.HintContextForLLM{}, err
	}
	if len(attempts) == 0 {
		return domain.HintContextForLLM{}, fmt.Errorf("hint context not found")
	}
	var task domain.TaskContext
	if attempts[len(attempts)-1].TaskJSON != "" {
		_ = json.Unmarshal([]byte(attempts[len(attempts)-1].TaskJSON), &task)
	}
	if task.TaskID == "" {
		task.TaskID = req.TaskID
	}
	hints, _ := r.loadHintEvents(ctx, req.UserID, req.TaskID)
	return domain.HintContextForLLM{
		UserID:               req.UserID,
		TaskID:               req.TaskID,
		CurrentAttemptNumber: req.CurrentAttemptNumber,
		Task:                 task,
		Attempts:             attempts,
		PreviousHints:        hints,
		PreparedAt:           time.Now().UTC(),
	}, nil
}

func (r *Repository) loadHintEvents(ctx context.Context, userID, taskID string) ([]domain.HintEventLog, error) {
	rows, err := r.conn.Query(ctx, `
		SELECT event_id, user_id, task_id, hint_id, event_type, hint_type, attempt_number, related_skills, message, request_json, response_json, created_at
		FROM hint_events
		WHERE user_id = ? AND task_id = ?
		ORDER BY created_at ASC`, userID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.HintEventLog{}
	for rows.Next() {
		var h domain.HintEventLog
		var attemptNumber int32
		if err := rows.Scan(&h.EventID, &h.UserID, &h.TaskID, &h.HintID, &h.EventType, &h.HintType, &attemptNumber, &h.RelatedSkills, &h.Message, &h.RequestJSON, &h.ResponseJSON, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.AttemptNumber = int(attemptNumber)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *Repository) StoreHintGenerated(ctx context.Context, eventID string, resp domain.HintGenerateResponse, requestJSON, responseJSON string) error {
	created := resp.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return r.conn.Exec(ctx, `
		INSERT INTO hint_events
		(event_id, user_id, task_id, hint_id, event_type, hint_type, attempt_number, related_skills, message, request_json, response_json, created_at, ingested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now64(3))`,
		eventID, resp.UserID, resp.TaskID, resp.HintID, domain.EventTypeAnalyticsHintGenerated, resp.HintType, int32(resp.AttemptNumber), resp.RelatedSkills, resp.Message, requestJSON, responseJSON, created)
}
