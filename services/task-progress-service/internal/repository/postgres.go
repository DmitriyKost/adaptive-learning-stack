package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"task-progress-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Ping(ctx context.Context) error {
	return r.db.Ping(ctx)
}

func (r *Repository) ListGraphs(ctx context.Context) ([]domain.LearningGraph, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, code, name, COALESCE(description, ''), is_active, created_at, updated_at
		FROM learning_graphs
		ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var graphs []domain.LearningGraph
	for rows.Next() {
		var g domain.LearningGraph
		if err := rows.Scan(&g.ID, &g.Code, &g.Name, &g.Description, &g.IsActive, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
	}
	return graphs, rows.Err()
}

func (r *Repository) GetGraphByCode(ctx context.Context, code string) (domain.LearningGraph, error) {
	var g domain.LearningGraph
	err := r.db.QueryRow(ctx, `
		SELECT id::text, code, name, COALESCE(description, ''), is_active, created_at, updated_at
		FROM learning_graphs
		WHERE code = $1 AND is_active = true`, code).Scan(&g.ID, &g.Code, &g.Name, &g.Description, &g.IsActive, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LearningGraph{}, domain.ErrNotFound
	}
	return g, err
}

func (r *Repository) GetGraphByID(ctx context.Context, id string) (domain.LearningGraph, error) {
	var g domain.LearningGraph
	err := r.db.QueryRow(ctx, `
		SELECT id::text, code, name, COALESCE(description, ''), is_active, created_at, updated_at
		FROM learning_graphs
		WHERE id = $1 AND is_active = true`, id).Scan(&g.ID, &g.Code, &g.Name, &g.Description, &g.IsActive, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.LearningGraph{}, domain.ErrNotFound
	}
	return g, err
}

func (r *Repository) CreateGraph(ctx context.Context, g domain.LearningGraph) (domain.LearningGraph, error) {
	if g.ID == "" {
		id, err := domain.NewUUID()
		if err != nil {
			return domain.LearningGraph{}, err
		}
		g.ID = id
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO learning_graphs (id, code, name, description, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, code, name, COALESCE(description, ''), is_active, created_at, updated_at`,
		g.ID, g.Code, g.Name, g.Description, g.IsActive,
	).Scan(&g.ID, &g.Code, &g.Name, &g.Description, &g.IsActive, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

func (r *Repository) UpsertGraphSkill(ctx context.Context, graphID, skillID string, position int, required bool, priorityWeight, masteryThreshold float64) error {
	if masteryThreshold <= 0 || masteryThreshold > 1 {
		masteryThreshold = 0.70
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO graph_skills (graph_id, skill_id, position, is_required, priority_weight, mastery_threshold)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (graph_id, skill_id) DO UPDATE
		SET position = EXCLUDED.position,
		    is_required = EXCLUDED.is_required,
		    priority_weight = EXCLUDED.priority_weight,
		    mastery_threshold = EXCLUDED.mastery_threshold`, graphID, skillID, position, required, priorityWeight, masteryThreshold)
	return err
}

func (r *Repository) UpsertDependency(ctx context.Context, graphID, skillID, dependsOnSkillID string, strength float64, requiredMastery *float64) error {
	if requiredMastery != nil && (*requiredMastery <= 0 || *requiredMastery > 1) {
		requiredMastery = nil
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO skill_dependencies (graph_id, skill_id, depends_on_skill_id, strength, required_mastery)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (graph_id, skill_id, depends_on_skill_id) DO UPDATE
		SET strength = EXCLUDED.strength,
		    required_mastery = EXCLUDED.required_mastery`, graphID, skillID, dependsOnSkillID, strength, requiredMastery)
	return err
}

func (r *Repository) GetOrCreateUserProfile(ctx context.Context, userID, defaultGraphCode string) (domain.UserLearningProfile, error) {
	profile, err := r.GetUserProfile(ctx, userID)
	if err == nil {
		return profile, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.UserLearningProfile{}, err
	}
	graph, err := r.GetGraphByCode(ctx, defaultGraphCode)
	if err != nil {
		return domain.UserLearningProfile{}, err
	}
	return r.SetUserProfile(ctx, userID, graph.ID, "core", "default graph")
}

func (r *Repository) GetUserProfile(ctx context.Context, userID string) (domain.UserLearningProfile, error) {
	var p domain.UserLearningProfile
	err := r.db.QueryRow(ctx, `
		SELECT p.user_id::text, p.graph_id::text, g.code, p.professional_track,
		       COALESCE(p.recommendation_reason, ''), p.created_at, p.updated_at
		FROM user_learning_profiles p
		JOIN learning_graphs g ON g.id = p.graph_id
		WHERE p.user_id = $1`, userID).Scan(&p.UserID, &p.GraphID, &p.GraphCode, &p.ProfessionalTrack, &p.RecommendationReason, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserLearningProfile{}, domain.ErrNotFound
	}
	return p, err
}

func (r *Repository) SetUserProfile(ctx context.Context, userID, graphID, track, reason string) (domain.UserLearningProfile, error) {
	var p domain.UserLearningProfile
	err := r.db.QueryRow(ctx, `
		INSERT INTO user_learning_profiles (user_id, graph_id, professional_track, recommendation_reason, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE
		SET graph_id = EXCLUDED.graph_id,
		    professional_track = EXCLUDED.professional_track,
		    recommendation_reason = EXCLUDED.recommendation_reason,
		    updated_at = now()
		RETURNING user_id::text, graph_id::text,
		          (SELECT code FROM learning_graphs WHERE id = user_learning_profiles.graph_id),
		          professional_track, COALESCE(recommendation_reason, ''), created_at, updated_at`,
		userID, graphID, track, reason,
	).Scan(&p.UserID, &p.GraphID, &p.GraphCode, &p.ProfessionalTrack, &p.RecommendationReason, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *Repository) ListGraphSkills(ctx context.Context, graphID string) ([]domain.GraphSkill, error) {
	rows, err := r.db.Query(ctx, `
		SELECT gs.graph_id::text, gs.skill_id::text, s.code, s.name, gs.position, gs.is_required, gs.priority_weight, gs.mastery_threshold, gs.created_at
		FROM graph_skills gs
		JOIN skills s ON s.id = gs.skill_id
		WHERE gs.graph_id = $1
		ORDER BY gs.position, s.code`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.GraphSkill
	for rows.Next() {
		var item domain.GraphSkill
		if err := rows.Scan(&item.GraphID, &item.SkillID, &item.SkillCode, &item.SkillName, &item.Position, &item.IsRequired, &item.PriorityWeight, &item.MasteryThreshold, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListDependencies(ctx context.Context, graphID string) ([]domain.SkillDependency, error) {
	rows, err := r.db.Query(ctx, `
		SELECT graph_id::text, skill_id::text, depends_on_skill_id::text, strength, required_mastery, created_at
		FROM skill_dependencies
		WHERE graph_id = $1`, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.SkillDependency
	for rows.Next() {
		var d domain.SkillDependency
		if err := rows.Scan(&d.GraphID, &d.SkillID, &d.DependsOnSkillID, &d.Strength, &d.RequiredMastery, &d.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *Repository) ListEffectiveGraphSkills(ctx context.Context, userID, graphID string) ([]domain.GraphSkill, error) {
	rows, err := r.db.Query(ctx, `
		WITH ref AS (
			SELECT gs.graph_id, gs.skill_id, s.code, s.name, gs.position, gs.is_required,
			       gs.priority_weight, gs.mastery_threshold, gs.created_at
			FROM graph_skills gs
			JOIN skills s ON s.id = gs.skill_id
			WHERE gs.graph_id = $2
		), ovr AS (
			SELECT ugso.graph_id, ugso.skill_id, s.code, s.name, ugso.position, ugso.is_required,
			       ugso.priority_weight, ugso.mastery_threshold, ugso.updated_at AS created_at
			FROM user_graph_skill_overrides ugso
			JOIN skills s ON s.id = ugso.skill_id
			WHERE ugso.user_id = $1 AND ugso.graph_id = $2
		), effective AS (
			SELECT ref.graph_id, ref.skill_id, ref.code, ref.name,
			       COALESCE(ovr.position, ref.position) AS position,
			       COALESCE(ovr.is_required, ref.is_required) AS is_required,
			       COALESCE(ovr.priority_weight, ref.priority_weight) AS priority_weight,
			       COALESCE(ovr.mastery_threshold, ref.mastery_threshold) AS mastery_threshold,
			       ref.created_at
			FROM ref
			LEFT JOIN ovr ON ovr.graph_id = ref.graph_id AND ovr.skill_id = ref.skill_id
			UNION ALL
			SELECT ovr.graph_id, ovr.skill_id, ovr.code, ovr.name,
			       COALESCE(ovr.position, 10000) AS position,
			       COALESCE(ovr.is_required, true) AS is_required,
			       COALESCE(ovr.priority_weight, 1.0) AS priority_weight,
			       COALESCE(ovr.mastery_threshold, 0.70) AS mastery_threshold,
			       ovr.created_at
			FROM ovr
			WHERE NOT EXISTS (SELECT 1 FROM ref WHERE ref.graph_id = ovr.graph_id AND ref.skill_id = ovr.skill_id)
		)
		SELECT graph_id::text, skill_id::text, code, name, position, is_required, priority_weight, mastery_threshold, created_at
		FROM effective
		ORDER BY position, code`, userID, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.GraphSkill
	for rows.Next() {
		var item domain.GraphSkill
		if err := rows.Scan(&item.GraphID, &item.SkillID, &item.SkillCode, &item.SkillName, &item.Position, &item.IsRequired, &item.PriorityWeight, &item.MasteryThreshold, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListEffectiveDependencies(ctx context.Context, userID, graphID string) ([]domain.SkillDependency, error) {
	rows, err := r.db.Query(ctx, `
		WITH ref AS (
			SELECT sd.graph_id, sd.skill_id, sd.depends_on_skill_id, sd.strength, sd.required_mastery, sd.created_at
			FROM skill_dependencies sd
			WHERE sd.graph_id = $2
		), ovr AS (
			SELECT usdo.graph_id, usdo.skill_id, usdo.depends_on_skill_id,
			       usdo.strength, usdo.required_mastery, usdo.updated_at AS created_at
			FROM user_skill_dependency_overrides usdo
			WHERE usdo.user_id = $1 AND usdo.graph_id = $2
		), effective AS (
			SELECT ref.graph_id, ref.skill_id, ref.depends_on_skill_id,
			       COALESCE(ovr.strength, ref.strength) AS strength,
			       COALESCE(ovr.required_mastery, ref.required_mastery) AS required_mastery,
			       ref.created_at
			FROM ref
			LEFT JOIN ovr ON ovr.graph_id = ref.graph_id AND ovr.skill_id = ref.skill_id AND ovr.depends_on_skill_id = ref.depends_on_skill_id
			UNION ALL
			SELECT ovr.graph_id, ovr.skill_id, ovr.depends_on_skill_id,
			       COALESCE(ovr.strength, 1.0) AS strength,
			       ovr.required_mastery,
			       ovr.created_at
			FROM ovr
			WHERE NOT EXISTS (
				SELECT 1 FROM ref WHERE ref.graph_id = ovr.graph_id AND ref.skill_id = ovr.skill_id AND ref.depends_on_skill_id = ovr.depends_on_skill_id
			)
		)
		SELECT e.graph_id::text, e.skill_id::text, s.code,
		       e.depends_on_skill_id::text, ds.code,
		       e.strength, e.required_mastery, e.created_at
		FROM effective e
		JOIN skills s ON s.id = e.skill_id
		JOIN skills ds ON ds.id = e.depends_on_skill_id`, userID, graphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.SkillDependency
	for rows.Next() {
		var d domain.SkillDependency
		if err := rows.Scan(&d.GraphID, &d.SkillID, &d.SkillCode, &d.DependsOnSkillID, &d.DependsOnSkillCode, &d.Strength, &d.RequiredMastery, &d.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *Repository) ListActiveRecommendations(ctx context.Context, userID, graphID string, now time.Time) ([]domain.UserSkillRecommendation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT usr.user_id::text, usr.graph_id::text, usr.skill_id::text, s.code,
		       usr.priority, COALESCE(usr.recommended_action, ''), COALESCE(usr.reason, ''),
		       COALESCE(usr.source_event_id::text, ''), usr.expires_at, usr.updated_at
		FROM user_skill_recommendations usr
		JOIN skills s ON s.id = usr.skill_id
		WHERE usr.user_id = $1 AND usr.graph_id = $2
		  AND (usr.expires_at IS NULL OR usr.expires_at > $3)
		ORDER BY usr.priority DESC, s.code`, userID, graphID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.UserSkillRecommendation
	for rows.Next() {
		var item domain.UserSkillRecommendation
		if err := rows.Scan(&item.UserID, &item.GraphID, &item.SkillID, &item.SkillCode, &item.Priority, &item.RecommendedAction, &item.Reason, &item.SourceEventID, &item.ExpiresAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetActiveAnalysisState(ctx context.Context, userID string, now time.Time) (*domain.UserAnalysisState, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE user_analysis_state
		SET status = 'expired', updated_at = $2
		WHERE user_id = $1 AND status = 'pending' AND wait_until IS NOT NULL AND wait_until <= $2`, userID, now)
	if err != nil {
		return nil, err
	}
	var state domain.UserAnalysisState
	err = r.db.QueryRow(ctx, `
		SELECT user_id::text, COALESCE(pending_task_id::text, ''), COALESCE(pending_attempt_id::text, ''),
		       status, pending_since, wait_until, COALESCE(source_event_id::text, ''), updated_at
		FROM user_analysis_state
		WHERE user_id = $1 AND status = 'pending'`, userID).Scan(&state.UserID, &state.PendingTaskID, &state.PendingAttemptID, &state.Status, &state.PendingSince, &state.WaitUntil, &state.SourceEventID, &state.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *Repository) GetNextTaskRecommendation(ctx context.Context, userID string, now time.Time) (domain.NextTaskRecommendation, error) {
	var entry domain.NextTaskCacheEntry
	var recommendedSkillsRaw []byte
	err := r.db.QueryRow(ctx, `
		SELECT user_id::text, status, COALESCE(task_id::text, ''), COALESCE(graph_id::text, ''),
		       COALESCE(reason, ''), score, COALESCE(graph_code, ''), COALESCE(professional_track, ''), repeat_mode,
		       recommended_skills, COALESCE(source_event_id::text, ''), COALESCE(source_attempt_id::text, ''),
		       COALESCE(source_analysis_run_id, ''), wait_until, expires_at, created_at, updated_at
		FROM user_next_task_recommendations
		WHERE user_id = $1`, userID).Scan(&entry.UserID, &entry.Status, &entry.TaskID, &entry.GraphID, &entry.Reason, &entry.Score, &entry.GraphCode, &entry.ProfessionalTrack, &entry.RepeatMode, &recommendedSkillsRaw, &entry.SourceEventID, &entry.SourceAttemptID, &entry.SourceAnalysisRunID, &entry.WaitUntil, &entry.ExpiresAt, &entry.CreatedAt, &entry.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NextTaskRecommendation{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}

	if entry.Status == domain.NextTaskStatusPending {
		retryAfter := retryAfterSeconds(entry.WaitUntil, now, 5)
		return domain.NextTaskRecommendation{}, &domain.AnalysisPendingError{State: domain.UserAnalysisState{UserID: userID, PendingTaskID: entry.TaskID, PendingAttemptID: entry.SourceAttemptID, Status: domain.AnalysisStatusPending, PendingSince: entry.UpdatedAt, WaitUntil: entry.WaitUntil, SourceEventID: entry.SourceEventID, UpdatedAt: entry.UpdatedAt}, RetryAfterSeconds: retryAfter}
	}
	if entry.Status != domain.NextTaskStatusReady || entry.TaskID == "" {
		return domain.NextTaskRecommendation{}, domain.ErrNotFound
	}
	if entry.ExpiresAt != nil && !entry.ExpiresAt.After(now) {
		return domain.NextTaskRecommendation{}, domain.ErrStaleAssessment
	}
	if len(recommendedSkillsRaw) > 0 {
		_ = json.Unmarshal(recommendedSkillsRaw, &entry.RecommendedSkills)
	}
	task, err := r.GetTaskByID(ctx, entry.TaskID)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	return domain.NextTaskRecommendation{Task: task, Reason: entry.Reason, Score: entry.Score, GraphCode: entry.GraphCode, Track: entry.ProfessionalTrack, RepeatMode: entry.RepeatMode, RecommendedSkills: entry.RecommendedSkills}, nil
}

func (r *Repository) StartNextTaskRefresh(ctx context.Context, userID string, now time.Time, waitTimeout time.Duration) (domain.UserAnalysisState, error) {
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	waitUntil := now.Add(waitTimeout)
	var state domain.UserAnalysisState
	err := r.db.QueryRow(ctx, `
		INSERT INTO user_next_task_recommendations (user_id, status, wait_until, updated_at)
		VALUES ($1, 'pending', $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET status = 'pending',
		    wait_until = EXCLUDED.wait_until,
		    updated_at = EXCLUDED.updated_at
		RETURNING user_id::text, COALESCE(task_id::text, ''), COALESCE(source_attempt_id::text, ''), status, updated_at, wait_until, COALESCE(source_event_id::text, ''), updated_at`, userID, waitUntil, now).Scan(&state.UserID, &state.PendingTaskID, &state.PendingAttemptID, &state.Status, &state.PendingSince, &state.WaitUntil, &state.SourceEventID, &state.UpdatedAt)
	return state, err
}

func (r *Repository) SaveNextTaskRecommendation(ctx context.Context, userID string, rec domain.NextTaskRecommendation, sourceEventID, sourceAttemptID, sourceAnalysisRunID string, expiresAt time.Time) error {
	if userID == "" || rec.Task.ID == "" {
		return domain.ErrInvalidInput
	}
	profile, err := r.GetUserProfile(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		profile, err = r.GetOrCreateUserProfile(ctx, userID, rec.GraphCode)
	}
	if err != nil {
		return err
	}
	if rec.GraphCode == "" {
		rec.GraphCode = profile.GraphCode
	}
	if rec.Track == "" {
		rec.Track = profile.ProfessionalTrack
	}
	recommendedSkills, err := json.Marshal(rec.RecommendedSkills)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = r.db.Exec(ctx, `
		INSERT INTO user_next_task_recommendations (user_id, status, task_id, graph_id, reason, score, graph_code, professional_track, repeat_mode, recommended_skills, source_event_id, source_attempt_id, source_analysis_run_id, expires_at, wait_until, created_at, updated_at)
		VALUES ($1, 'ready', $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9::jsonb, $10, $11, NULLIF($12, ''), $13, NULL, $14, $14)
		ON CONFLICT (user_id) DO UPDATE
		SET status = 'ready',
		    task_id = EXCLUDED.task_id,
		    graph_id = EXCLUDED.graph_id,
		    reason = EXCLUDED.reason,
		    score = EXCLUDED.score,
		    graph_code = EXCLUDED.graph_code,
		    professional_track = EXCLUDED.professional_track,
		    repeat_mode = EXCLUDED.repeat_mode,
		    recommended_skills = EXCLUDED.recommended_skills,
		    source_event_id = EXCLUDED.source_event_id,
		    source_attempt_id = EXCLUDED.source_attempt_id,
		    source_analysis_run_id = EXCLUDED.source_analysis_run_id,
		    expires_at = EXCLUDED.expires_at,
		    wait_until = NULL,
		    updated_at = EXCLUDED.updated_at`, userID, rec.Task.ID, profile.GraphID, rec.Reason, rec.Score, rec.GraphCode, rec.Track, rec.RepeatMode, string(recommendedSkills), nullUUID(sourceEventID), nullUUID(sourceAttemptID), sourceAnalysisRunID, expiresAt, now)
	return err
}

func (r *Repository) MarkNextTaskRefreshFailed(ctx context.Context, userID string, now time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_next_task_recommendations (user_id, status, updated_at)
		VALUES ($1, 'failed', $2)
		ON CONFLICT (user_id) DO UPDATE
		SET status = 'failed', wait_until = NULL, updated_at = EXCLUDED.updated_at`, userID, now)
	return err
}

func retryAfterSeconds(waitUntil *time.Time, now time.Time, fallback int) int {
	if fallback <= 0 {
		fallback = 5
	}
	if waitUntil == nil {
		return fallback
	}
	remaining := int(waitUntil.Sub(now).Seconds())
	if remaining <= 0 {
		return 1
	}
	if remaining < fallback {
		return remaining
	}
	return fallback
}

func (r *Repository) ListTasks(ctx context.Context) ([]domain.Task, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, title, description, difficulty, COALESCE(reference_sql, ''), COALESCE(dataset_id::text, ''), is_active, created_at, updated_at
		FROM tasks
		WHERE is_active = true
		ORDER BY created_at, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Difficulty, &t.ReferenceSQL, &t.DatasetID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.attachTaskSkills(ctx, tasks)
}

func (r *Repository) GetTaskByID(ctx context.Context, taskID string) (domain.Task, error) {
	var t domain.Task
	err := r.db.QueryRow(ctx, `
		SELECT id::text, title, description, difficulty, COALESCE(reference_sql, ''), COALESCE(dataset_id::text, ''), is_active, created_at, updated_at
		FROM tasks
		WHERE id = $1`, taskID).Scan(&t.ID, &t.Title, &t.Description, &t.Difficulty, &t.ReferenceSQL, &t.DatasetID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	tasks, err := r.attachTaskSkills(ctx, []domain.Task{t})
	if err != nil {
		return domain.Task{}, err
	}
	return tasks[0], nil
}

func (r *Repository) attachTaskSkills(ctx context.Context, tasks []domain.Task) ([]domain.Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	ids := make([]string, 0, len(tasks))
	byID := make(map[string]int, len(tasks))
	for i := range tasks {
		ids = append(ids, tasks[i].ID)
		byID[tasks[i].ID] = i
	}

	rows, err := r.db.Query(ctx, `
		SELECT ts.task_id::text, ts.skill_id::text, s.code, ts.weight
		FROM task_skills ts
		JOIN skills s ON s.id = ts.skill_id
		WHERE ts.task_id::text = ANY($1)
		ORDER BY s.code`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ts domain.TaskSkill
		if err := rows.Scan(&ts.TaskID, &ts.SkillID, &ts.SkillCode, &ts.Weight); err != nil {
			return nil, err
		}
		if idx, ok := byID[ts.TaskID]; ok {
			tasks[idx].Skills = append(tasks[idx].Skills, ts)
		}
	}
	return tasks, rows.Err()
}

func (r *Repository) GetUserSkills(ctx context.Context, userID string) ([]domain.UserSkill, error) {
	rows, err := r.db.Query(ctx, `
		SELECT us.user_id::text, us.skill_id::text, s.code, s.name,
		       us.mastery_score, us.confidence, us.attempts_count, us.success_count,
		       us.last_used_at, us.updated_at
		FROM user_skills us
		JOIN skills s ON s.id = us.skill_id
		WHERE us.user_id = $1
		ORDER BY s.code`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.UserSkill
	for rows.Next() {
		var us domain.UserSkill
		if err := rows.Scan(&us.UserID, &us.SkillID, &us.SkillCode, &us.SkillName, &us.MasteryScore, &us.Confidence, &us.AttemptsCount, &us.SuccessCount, &us.LastUsedAt, &us.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, us)
	}
	return result, rows.Err()
}

func (r *Repository) GetUserSkillsMap(ctx context.Context, userID string) (map[string]domain.UserSkill, error) {
	skills, err := r.GetUserSkills(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.UserSkill, len(skills))
	for _, skill := range skills {
		result[skill.SkillID] = skill
	}
	return result, nil
}

func (r *Repository) GetSolvedTaskIDs(ctx context.Context, userID string) (map[string]domain.UserTaskStatus, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id::text, task_id::text, status, attempts_count, solved_at, last_attempt_at, COALESCE(last_attempt_id::text, '')
		FROM user_task_status
		WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]domain.UserTaskStatus)
	for rows.Next() {
		var s domain.UserTaskStatus
		if err := rows.Scan(&s.UserID, &s.TaskID, &s.Status, &s.AttemptsCount, &s.SolvedAt, &s.LastAttemptAt, &s.LastAttemptID); err != nil {
			return nil, err
		}
		result[s.TaskID] = s
	}
	return result, rows.Err()
}

func (r *Repository) CreateAttemptAndUpdateModel(ctx context.Context, attempt domain.TaskAttempt, task domain.Task, deltas []SkillDelta, sourceEventID string, recommendationWaitTimeout time.Duration) (int, []SkillUpdateResult, *domain.UserAnalysisState, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if sourceEventID != "" {
		inserted, err := markEventProcessedTx(ctx, tx, sourceEventID, "playground.execution.completed")
		if err != nil {
			return 0, nil, nil, err
		}
		if !inserted {
			return 0, nil, nil, domain.ErrDuplicateEvent
		}
	}

	status := domain.TaskStatusInProgress
	var solvedAt *time.Time
	if attempt.IsCorrect {
		status = domain.TaskStatusSolved
		now := attempt.CreatedAt
		solvedAt = &now
	}

	var attemptNumber int
	err = tx.QueryRow(ctx, `
		INSERT INTO user_task_status (user_id, task_id, status, attempts_count, solved_at, last_attempt_at, last_attempt_id)
		VALUES ($1, $2, $3, 1, $4, $5, $6)
		ON CONFLICT (user_id, task_id) DO UPDATE
		SET attempts_count = user_task_status.attempts_count + 1,
		    status = CASE WHEN EXCLUDED.status = 'solved' THEN 'solved' ELSE user_task_status.status END,
		    solved_at = CASE WHEN EXCLUDED.status = 'solved' THEN EXCLUDED.solved_at ELSE user_task_status.solved_at END,
		    last_attempt_at = EXCLUDED.last_attempt_at,
		    last_attempt_id = EXCLUDED.last_attempt_id
		RETURNING attempts_count`, attempt.UserID, attempt.TaskID, status, solvedAt, attempt.CreatedAt, attempt.ID).Scan(&attemptNumber)
	if err != nil {
		return 0, nil, nil, err
	}

	updates := make([]SkillUpdateResult, 0, len(deltas))
	for _, d := range deltas {
		update, err := upsertUserSkillTx(ctx, tx, attempt.UserID, d.SkillID, d.Delta, attempt.IsCorrect, 0.05, "task_attempt", attempt.CreatedAt)
		if err != nil {
			return 0, nil, nil, err
		}
		update.SkillCode = d.SkillCode
		updates = append(updates, update)
	}

	var analysisState *domain.UserAnalysisState
	if attempt.IsCorrect {
		state, err := setAnalysisPendingTx(ctx, tx, attempt.UserID, attempt.TaskID, attempt.ID, sourceEventID, attempt.CreatedAt, recommendationWaitTimeout)
		if err != nil {
			return 0, nil, nil, err
		}
		if err := setNextTaskPendingTx(ctx, tx, attempt.UserID, attempt.TaskID, attempt.ID, sourceEventID, attempt.CreatedAt, recommendationWaitTimeout); err != nil {
			return 0, nil, nil, err
		}
		analysisState = &state
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, nil, nil, err
	}
	return attemptNumber, updates, analysisState, nil
}

type SkillDelta struct {
	SkillID   string
	SkillCode string
	Weight    float64
	Delta     float64
}

type SkillUpdateResult struct {
	SkillID       string
	SkillCode     string
	OldMastery    float64
	NewMastery    float64
	OldConfidence float64
	NewConfidence float64
}

func upsertUserSkillTx(ctx context.Context, tx pgx.Tx, userID, skillID string, delta float64, success bool, confidenceDelta float64, source string, now time.Time) (SkillUpdateResult, error) {
	var current domain.UserSkill
	err := tx.QueryRow(ctx, `
		SELECT user_id::text, skill_id::text, mastery_score, confidence, attempts_count, success_count, last_used_at, updated_at
		FROM user_skills
		WHERE user_id = $1 AND skill_id = $2
		FOR UPDATE`, userID, skillID).Scan(&current.UserID, &current.SkillID, &current.MasteryScore, &current.Confidence, &current.AttemptsCount, &current.SuccessCount, &current.LastUsedAt, &current.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		oldMastery := 0.0
		oldConfidence := 0.0
		newMastery := clamp(delta, 0, 1)
		newConfidence := clamp(confidenceDelta, 0, 1)
		successCount := 0
		if success {
			successCount = 1
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO user_skills (user_id, skill_id, mastery_score, confidence, attempts_count, success_count, last_used_at, updated_at)
			VALUES ($1, $2, $3, $4, 1, $5, $6, $6)`, userID, skillID, newMastery, newConfidence, successCount, now)
		return SkillUpdateResult{SkillID: skillID, OldMastery: oldMastery, NewMastery: newMastery, OldConfidence: oldConfidence, NewConfidence: newConfidence}, err
	}
	if err != nil {
		return SkillUpdateResult{}, err
	}

	newMastery := clamp(current.MasteryScore+delta, 0, 1)
	newConfidence := clamp(current.Confidence+confidenceDelta, 0, 1)
	successInc := 0
	if success {
		successInc = 1
	}
	_, err = tx.Exec(ctx, `
		UPDATE user_skills
		SET mastery_score = $3,
		    confidence = $4,
		    attempts_count = attempts_count + 1,
		    success_count = success_count + $5,
		    last_used_at = $6,
		    updated_at = $6
		WHERE user_id = $1 AND skill_id = $2`, userID, skillID, newMastery, newConfidence, successInc, now)
	if err != nil {
		return SkillUpdateResult{}, err
	}
	return SkillUpdateResult{SkillID: skillID, OldMastery: current.MasteryScore, NewMastery: newMastery, OldConfidence: current.Confidence, NewConfidence: newConfidence}, nil
}

func (r *Repository) ApplyAnalyticsAssessment(ctx context.Context, event domain.AnalyticsSkillAssessmentUpdatedEvent) ([]SkillUpdateResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inserted, err := markEventProcessedTx(ctx, tx, event.EventID, "analytics.skill_assessment.updated")
	if err != nil {
		return nil, err
	}
	if !inserted {
		return nil, domain.ErrDuplicateEvent
	}

	graphID := event.GraphID
	if graphID == "" && event.GraphCode != "" {
		err = tx.QueryRow(ctx, `SELECT id::text FROM learning_graphs WHERE code = $1 AND is_active = true`, event.GraphCode).Scan(&graphID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	}

	overlay := event.UserGraphOverlay
	if overlay == nil {
		overlay = event.GraphPatch
	}
	if overlay != nil {
		patchedGraphID, patchedGraphCode, err := applyUserGraphOverlayTx(ctx, tx, event.UserID, *overlay, event.EventID, event.CreatedAt)
		if err != nil {
			return nil, err
		}
		if graphID == "" {
			graphID = patchedGraphID
		}
		if event.GraphCode == "" {
			event.GraphCode = patchedGraphCode
		}
		if event.ProfessionalTrack == "" && overlay.ProfessionalTrack != "" {
			event.ProfessionalTrack = overlay.ProfessionalTrack
		}
	}

	if graphID != "" || event.GraphCode != "" {
		if graphID == "" {
			err = tx.QueryRow(ctx, `SELECT id::text FROM learning_graphs WHERE code = $1 AND is_active = true`, event.GraphCode).Scan(&graphID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, domain.ErrNotFound
			}
			if err != nil {
				return nil, err
			}
		}
		track := event.ProfessionalTrack
		if track == "" {
			track = event.GraphCode
		}
		if track == "" {
			track = "recommended"
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO user_learning_profiles (user_id, graph_id, professional_track, recommendation_reason, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (user_id) DO UPDATE
			SET graph_id = EXCLUDED.graph_id,
			    professional_track = EXCLUDED.professional_track,
			    recommendation_reason = EXCLUDED.recommendation_reason,
			    updated_at = EXCLUDED.updated_at`, event.UserID, graphID, track, event.RecommendationReason, event.CreatedAt)
		if err != nil {
			return nil, err
		}
	}

	if graphID == "" {
		var profileGraphID string
		err = tx.QueryRow(ctx, `SELECT graph_id::text FROM user_learning_profiles WHERE user_id = $1`, event.UserID).Scan(&profileGraphID)
		if err == nil {
			graphID = profileGraphID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	if graphID != "" {
		// Recommended skills from analytics/LLM are treated as a full snapshot for
		// the active graph. Old recommendations are removed before inserting the
		// newest batch, so the planner never aggregates stale tag recommendations.
		if err := replaceRecommendedSkillsSnapshotTx(ctx, tx, event.UserID, graphID, event.EventID, event.CreatedAt, event.RecommendedSkills); err != nil {
			return nil, err
		}
	}

	observedUntil := event.CreatedAt
	if event.ObservedUntil != nil && !event.ObservedUntil.IsZero() {
		observedUntil = *event.ObservedUntil
	}
	assessmentMode := event.AssessmentMode
	if assessmentMode == "" {
		assessmentMode = "merge"
	}

	updates := make([]SkillUpdateResult, 0, len(event.SkillScores))
	for _, assessment := range event.SkillScores {
		skillID := assessment.SkillID
		skillCode := assessment.SkillCode
		if skillID == "" {
			skillCode = canonicalSkillCode(assessment.SkillCode)
			err = tx.QueryRow(ctx, `SELECT id::text FROM skills WHERE code = $1`, skillCode).Scan(&skillID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
		} else if skillCode == "" {
			_ = tx.QueryRow(ctx, `SELECT code FROM skills WHERE id = $1`, skillID).Scan(&skillCode)
		}

		fresh, err := registerAssessmentVersionTx(ctx, tx, event.UserID, skillID, observedUntil, event.Analysis)
		if err != nil {
			return nil, err
		}
		if !fresh {
			continue
		}

		update, err := applyAssessmentTx(ctx, tx, event.UserID, skillID, assessment, assessmentMode, event.CreatedAt)
		if err != nil {
			return nil, err
		}
		update.SkillCode = skillCode
		updates = append(updates, update)
	}

	if err := completeAnalysisIfMatchesTx(ctx, tx, event.UserID, event.SourceTaskID, event.SourceAttemptID, event.EventID, event.CreatedAt); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return updates, nil
}

func applyUserGraphOverlayTx(ctx context.Context, tx pgx.Tx, userID string, overlay domain.GraphPatch, sourceEventID string, now time.Time) (string, string, error) {
	if overlay.GraphCode == "" {
		overlay.GraphCode = overlay.Code
	}
	if userID == "" || (overlay.GraphID == "" && overlay.GraphCode == "") {
		return "", "", domain.ErrInvalidInput
	}

	graphID := overlay.GraphID
	graphCode := overlay.GraphCode
	if graphID == "" {
		err := tx.QueryRow(ctx, `SELECT id::text, code FROM learning_graphs WHERE code = $1 AND is_active = true`, graphCode).Scan(&graphID, &graphCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", domain.ErrNotFound
		}
		if err != nil {
			return "", "", err
		}
	} else {
		err := tx.QueryRow(ctx, `SELECT id::text, code FROM learning_graphs WHERE id = $1 AND is_active = true`, graphID).Scan(&graphID, &graphCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", domain.ErrNotFound
		}
		if err != nil {
			return "", "", err
		}
	}

	for _, skill := range overlay.Skills {
		skillID, err := resolveSkillRefTx(ctx, tx, skill.SkillID, skill.SkillCode)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		var priority *float64
		if skill.PriorityWeight != nil && *skill.PriorityWeight >= 0 {
			priority = skill.PriorityWeight
		}
		var threshold *float64
		if skill.MasteryThreshold != nil && *skill.MasteryThreshold > 0 && *skill.MasteryThreshold <= 1 {
			threshold = skill.MasteryThreshold
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO user_graph_skill_overrides (user_id, graph_id, skill_id, position, is_required, priority_weight, mastery_threshold, reason, source_event_id, updated_at)
			VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6, $7, NULLIF($8, ''), $9, $10)
			ON CONFLICT (user_id, graph_id, skill_id) DO UPDATE
			SET position = COALESCE(EXCLUDED.position, user_graph_skill_overrides.position),
			    is_required = COALESCE(EXCLUDED.is_required, user_graph_skill_overrides.is_required),
			    priority_weight = COALESCE(EXCLUDED.priority_weight, user_graph_skill_overrides.priority_weight),
			    mastery_threshold = COALESCE(EXCLUDED.mastery_threshold, user_graph_skill_overrides.mastery_threshold),
			    reason = COALESCE(EXCLUDED.reason, user_graph_skill_overrides.reason),
			    source_event_id = EXCLUDED.source_event_id,
			    updated_at = EXCLUDED.updated_at`, userID, graphID, skillID, skill.Position, skill.IsRequired, priority, threshold, skill.Reason, nullUUID(sourceEventID), now)
		if err != nil {
			return "", "", err
		}
	}

	for _, dep := range overlay.Dependencies {
		skillID, err := resolveSkillRefTx(ctx, tx, dep.SkillID, dep.SkillCode)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		dependsOnSkillID, err := resolveSkillRefTx(ctx, tx, dep.DependsOnSkillID, dep.DependsOnSkillCode)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		var strength *float64
		if dep.Strength != nil && *dep.Strength > 0 {
			strength = dep.Strength
		}
		if dep.RequiredMastery != nil && (*dep.RequiredMastery <= 0 || *dep.RequiredMastery > 1) {
			dep.RequiredMastery = nil
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO user_skill_dependency_overrides (user_id, graph_id, skill_id, depends_on_skill_id, strength, required_mastery, reason, source_event_id, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9)
			ON CONFLICT (user_id, graph_id, skill_id, depends_on_skill_id) DO UPDATE
			SET strength = COALESCE(EXCLUDED.strength, user_skill_dependency_overrides.strength),
			    required_mastery = COALESCE(EXCLUDED.required_mastery, user_skill_dependency_overrides.required_mastery),
			    reason = COALESCE(EXCLUDED.reason, user_skill_dependency_overrides.reason),
			    source_event_id = EXCLUDED.source_event_id,
			    updated_at = EXCLUDED.updated_at`, userID, graphID, skillID, dependsOnSkillID, strength, dep.RequiredMastery, dep.Reason, nullUUID(sourceEventID), now)
		if err != nil {
			return "", "", err
		}
	}

	return graphID, graphCode, nil
}

func ensureSkillInGraphTx(ctx context.Context, tx pgx.Tx, graphID, skillID string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM graph_skills WHERE graph_id = $1 AND skill_id = $2)`, graphID, skillID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

func ensureDependencyInGraphTx(ctx context.Context, tx pgx.Tx, graphID, skillID, dependsOnSkillID string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM skill_dependencies WHERE graph_id = $1 AND skill_id = $2 AND depends_on_skill_id = $3)`, graphID, skillID, dependsOnSkillID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

func resolveSkillRefTx(ctx context.Context, tx pgx.Tx, skillID, skillCode string) (string, error) {
	if skillID != "" {
		return skillID, nil
	}
	skillCode = canonicalSkillCode(skillCode)
	if skillCode == "" {
		return "", domain.ErrInvalidInput
	}
	err := tx.QueryRow(ctx, `SELECT id::text FROM skills WHERE code = $1`, skillCode).Scan(&skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return skillID, err
}

func canonicalSkillCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	code = strings.ReplaceAll(code, "-", "_")
	code = strings.Join(strings.Fields(code), "_")

	switch code {
	case "join", "joins", "innerjoin", "inner_join_clause":
		return "inner_join"
	case "leftjoin", "left_join_clause":
		return "left_join"
	case "window", "window_function", "window_functions_clause":
		return "window_functions"
	case "aggregate", "aggregates", "aggregation", "aggregate_function", "aggregate_functions_clause":
		return "aggregate_functions"
	case "order", "orderby", "ordering", "sort", "sorting", "order_by_clause":
		return "order_by"
	case "group", "groupby", "grouping", "group_by_clause":
		return "group_by"
	case "filter", "filters", "filtering", "where_clause":
		return "where"
	case "select_statement", "projection":
		return "select"
	default:
		return code
	}
}

func replaceRecommendedSkillsSnapshotTx(ctx context.Context, tx pgx.Tx, userID, graphID, sourceEventID string, now time.Time, items []domain.RecommendedSkill) error {
	if userID == "" || graphID == "" {
		return domain.ErrInvalidInput
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_skill_recommendations WHERE user_id = $1 AND graph_id = $2`, userID, graphID); err != nil {
		return err
	}
	for _, rec := range items {
		skillID, err := resolveSkillRefTx(ctx, tx, rec.SkillID, rec.SkillCode)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		priority := rec.Priority
		if priority < 0 {
			priority = 0
		}
		if priority > 1 {
			priority = 1
		}
		expiresAt := rec.ExpiresAt
		if expiresAt == nil {
			defaultExpiresAt := now.Add(7 * 24 * time.Hour)
			expiresAt = &defaultExpiresAt
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO user_skill_recommendations (user_id, graph_id, skill_id, priority, recommended_action, reason, source_event_id, expires_at, updated_at)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, $9)
			ON CONFLICT (user_id, graph_id, skill_id) DO UPDATE
			SET priority = EXCLUDED.priority,
			    recommended_action = EXCLUDED.recommended_action,
			    reason = EXCLUDED.reason,
			    source_event_id = EXCLUDED.source_event_id,
			    expires_at = EXCLUDED.expires_at,
			    updated_at = EXCLUDED.updated_at`, userID, graphID, skillID, priority, rec.RecommendedAction, rec.Reason, nullUUID(sourceEventID), expiresAt, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func registerAssessmentVersionTx(ctx context.Context, tx pgx.Tx, userID, skillID string, observedUntil time.Time, meta *domain.AnalysisMetadata) (bool, error) {
	var analysisRunID, modelVersion, promptVersion string
	if meta != nil {
		analysisRunID = meta.AnalysisRunID
		modelVersion = meta.ModelVersion
		promptVersion = meta.PromptVersion
	}
	cmd, err := tx.Exec(ctx, `
		INSERT INTO user_skill_assessment_versions (user_id, skill_id, last_observed_until, last_analysis_run_id, model_version, prompt_version, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), now())
		ON CONFLICT (user_id, skill_id) DO UPDATE
		SET last_observed_until = EXCLUDED.last_observed_until,
		    last_analysis_run_id = EXCLUDED.last_analysis_run_id,
		    model_version = EXCLUDED.model_version,
		    prompt_version = EXCLUDED.prompt_version,
		    updated_at = now()
		WHERE user_skill_assessment_versions.last_observed_until <= EXCLUDED.last_observed_until`, userID, skillID, observedUntil, analysisRunID, modelVersion, promptVersion)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() == 1, nil
}

func setAnalysisPendingTx(ctx context.Context, tx pgx.Tx, userID, taskID, attemptID, sourceEventID string, now time.Time, waitTimeout time.Duration) (domain.UserAnalysisState, error) {
	waitUntil := now.Add(waitTimeout)
	var state domain.UserAnalysisState
	err := tx.QueryRow(ctx, `
		INSERT INTO user_analysis_state (user_id, pending_task_id, pending_attempt_id, status, pending_since, wait_until, source_event_id, updated_at)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6, $4)
		ON CONFLICT (user_id) DO UPDATE
		SET pending_task_id = EXCLUDED.pending_task_id,
		    pending_attempt_id = EXCLUDED.pending_attempt_id,
		    status = 'pending',
		    pending_since = EXCLUDED.pending_since,
		    wait_until = EXCLUDED.wait_until,
		    source_event_id = EXCLUDED.source_event_id,
		    updated_at = EXCLUDED.updated_at
		RETURNING user_id::text, COALESCE(pending_task_id::text, ''), COALESCE(pending_attempt_id::text, ''), status, pending_since, wait_until, COALESCE(source_event_id::text, ''), updated_at`, userID, taskID, nullUUID(attemptID), now, waitUntil, nullUUID(sourceEventID)).Scan(&state.UserID, &state.PendingTaskID, &state.PendingAttemptID, &state.Status, &state.PendingSince, &state.WaitUntil, &state.SourceEventID, &state.UpdatedAt)
	return state, err
}

func setNextTaskPendingTx(ctx context.Context, tx pgx.Tx, userID, taskID, attemptID, sourceEventID string, now time.Time, waitTimeout time.Duration) error {
	if waitTimeout <= 0 {
		waitTimeout = 30 * time.Second
	}
	waitUntil := now.Add(waitTimeout)
	_, err := tx.Exec(ctx, `
		INSERT INTO user_next_task_recommendations (user_id, status, task_id, source_attempt_id, source_event_id, wait_until, updated_at)
		VALUES ($1, 'pending', $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE
		SET status = 'pending',
		    task_id = EXCLUDED.task_id,
		    source_attempt_id = EXCLUDED.source_attempt_id,
		    source_event_id = EXCLUDED.source_event_id,
		    wait_until = EXCLUDED.wait_until,
		    updated_at = EXCLUDED.updated_at`, userID, taskID, nullUUID(attemptID), nullUUID(sourceEventID), waitUntil, now)
	return err
}

func completeAnalysisIfMatchesTx(ctx context.Context, tx pgx.Tx, userID, taskID, attemptID, sourceEventID string, now time.Time) error {
	if userID == "" {
		return domain.ErrInvalidInput
	}
	if taskID != "" && attemptID != "" {
		_, err := tx.Exec(ctx, `
			UPDATE user_analysis_state
			SET status = 'completed', wait_until = NULL, source_event_id = $4, updated_at = $5
			WHERE user_id = $1 AND status = 'pending' AND pending_task_id = $2 AND pending_attempt_id = $3`, userID, taskID, attemptID, nullUUID(sourceEventID), now)
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE user_analysis_state
		SET status = 'completed', wait_until = NULL, source_event_id = $2, updated_at = $3
		WHERE user_id = $1 AND status = 'pending'`, userID, nullUUID(sourceEventID), now)
	return err
}

func nullUUID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func applyAssessmentTx(ctx context.Context, tx pgx.Tx, userID, skillID string, assessment domain.SkillAssessment, mode string, now time.Time) (SkillUpdateResult, error) {
	var current domain.UserSkill
	err := tx.QueryRow(ctx, `
		SELECT user_id::text, skill_id::text, mastery_score, confidence, attempts_count, success_count, last_used_at, updated_at
		FROM user_skills
		WHERE user_id = $1 AND skill_id = $2
		FOR UPDATE`, userID, skillID).Scan(&current.UserID, &current.SkillID, &current.MasteryScore, &current.Confidence, &current.AttemptsCount, &current.SuccessCount, &current.LastUsedAt, &current.UpdatedAt)

	oldMastery := 0.0
	oldConfidence := 0.0
	newMastery := 0.0
	newConfidence := 0.0
	if errors.Is(err, pgx.ErrNoRows) {
		if assessment.MasteryScore != nil {
			newMastery = clamp(*assessment.MasteryScore, 0, 1)
		}
		if assessment.Delta != nil {
			newMastery = clamp(newMastery+*assessment.Delta, 0, 1)
		}
		if assessment.Confidence != nil {
			newConfidence = clamp(*assessment.Confidence, 0, 1)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO user_skills (user_id, skill_id, mastery_score, confidence, attempts_count, success_count, last_used_at, updated_at)
			VALUES ($1, $2, $3, $4, 0, 0, NULL, $5)`, userID, skillID, newMastery, newConfidence, now)
		return SkillUpdateResult{SkillID: skillID, OldMastery: oldMastery, NewMastery: newMastery, OldConfidence: oldConfidence, NewConfidence: newConfidence}, err
	}
	if err != nil {
		return SkillUpdateResult{}, err
	}

	oldMastery = current.MasteryScore
	oldConfidence = current.Confidence
	newMastery = oldMastery
	newConfidence = oldConfidence

	switch mode {
	case "replace":
		if assessment.MasteryScore != nil {
			newMastery = clamp(*assessment.MasteryScore, 0, 1)
		}
		if assessment.Delta != nil {
			newMastery = clamp(newMastery+*assessment.Delta, 0, 1)
		}
	case "delta":
		if assessment.Delta != nil {
			newMastery = clamp(oldMastery+*assessment.Delta, 0, 1)
		} else if assessment.MasteryScore != nil {
			newMastery = clamp(oldMastery+(*assessment.MasteryScore-oldMastery)*0.25, 0, 1)
		}
	default: // merge
		if assessment.MasteryScore != nil {
			confidence := 0.5
			if assessment.Confidence != nil {
				confidence = clamp(*assessment.Confidence, 0, 1)
			}
			newMastery = clamp(oldMastery*(1-confidence)+(*assessment.MasteryScore)*confidence, 0, 1)
		}
		if assessment.Delta != nil {
			newMastery = clamp(newMastery+*assessment.Delta, 0, 1)
		}
	}
	if assessment.Confidence != nil {
		newConfidence = clamp(*assessment.Confidence, 0, 1)
	}

	_, err = tx.Exec(ctx, `
		UPDATE user_skills
		SET mastery_score = $3,
		    confidence = $4,
		    updated_at = $5
		WHERE user_id = $1 AND skill_id = $2`, userID, skillID, newMastery, newConfidence, now)
	return SkillUpdateResult{SkillID: skillID, OldMastery: oldMastery, NewMastery: newMastery, OldConfidence: oldConfidence, NewConfidence: newConfidence}, err
}

func markEventProcessedTx(ctx context.Context, tx pgx.Tx, eventID, consumer string) (bool, error) {
	if eventID == "" {
		return false, domain.ErrInvalidInput
	}
	cmd, err := tx.Exec(ctx, `
		INSERT INTO processed_events (event_id, consumer, processed_at)
		VALUES ($1, $2, now())
		ON CONFLICT (event_id, consumer) DO NOTHING`, eventID, consumer)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() == 1, nil
}

func (r *Repository) CountUserAttempts(ctx context.Context, userID string) (completed int, attempts int, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT
		  COALESCE((SELECT count(*) FROM user_task_status WHERE user_id = $1 AND status = 'solved'), 0),
		  COALESCE((SELECT sum(attempts_count)::int FROM user_task_status WHERE user_id = $1), 0)`, userID).Scan(&completed, &attempts)
	return completed, attempts, err
}

func (r *Repository) CountUserGraphAttempts(ctx context.Context, userID, graphID string) (completed int, attempts int, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT
		  COALESCE(count(*) FILTER (WHERE uts.status = 'solved'), 0)::int AS completed,
		  COALESCE(sum(uts.attempts_count), 0)::int AS attempts
		FROM user_task_status uts
		WHERE uts.user_id = $1
		  AND EXISTS (
		    SELECT 1
		    FROM task_skills ts
		    JOIN graph_skills gs ON gs.skill_id = ts.skill_id
		    WHERE ts.task_id = uts.task_id AND gs.graph_id = $2
		  )`, userID, graphID).Scan(&completed, &attempts)
	return completed, attempts, err
}

func (r *Repository) ListSkills(ctx context.Context) ([]domain.Skill, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, code, name, COALESCE(description, ''), COALESCE(domain, ''), created_at, updated_at
		FROM skills
		ORDER BY domain, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Skill
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Description, &s.Domain, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *Repository) CreateSkill(ctx context.Context, skill domain.Skill) (domain.Skill, error) {
	if skill.ID == "" {
		id, err := domain.NewUUID()
		if err != nil {
			return domain.Skill{}, err
		}
		skill.ID = id
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO skills (id, code, name, description, domain, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), now())
		RETURNING id::text, code, name, COALESCE(description, ''), COALESCE(domain, ''), created_at, updated_at`,
		skill.ID, skill.Code, skill.Name, skill.Description, skill.Domain,
	).Scan(&skill.ID, &skill.Code, &skill.Name, &skill.Description, &skill.Domain, &skill.CreatedAt, &skill.UpdatedAt)
	return skill, err
}

func (r *Repository) FindSkillByCode(ctx context.Context, code string) (domain.Skill, error) {
	var s domain.Skill
	err := r.db.QueryRow(ctx, `
		SELECT id::text, code, name, COALESCE(description, ''), COALESCE(domain, ''), created_at, updated_at
		FROM skills WHERE code = $1`, code).Scan(&s.ID, &s.Code, &s.Name, &s.Description, &s.Domain, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Skill{}, domain.ErrNotFound
	}
	return s, err
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func NullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func WrapNotFound(err error, message string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", message, domain.ErrNotFound)
	}
	return err
}

func (r *Repository) GetUserTaskStatus(ctx context.Context, userID, taskID string) (domain.UserTaskStatus, error) {
	var s domain.UserTaskStatus
	err := r.db.QueryRow(ctx, `
		SELECT user_id::text, task_id::text, status, attempts_count, solved_at, last_attempt_at, COALESCE(last_attempt_id::text, '')
		FROM user_task_status
		WHERE user_id = $1 AND task_id = $2`, userID, taskID).Scan(&s.UserID, &s.TaskID, &s.Status, &s.AttemptsCount, &s.SolvedAt, &s.LastAttemptAt, &s.LastAttemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserTaskStatus{}, domain.ErrHintUnavailable
	}
	if err != nil {
		return domain.UserTaskStatus{}, err
	}
	return s, nil
}

func (r *Repository) GetHintState(ctx context.Context, userID, taskID string) (*domain.UserTaskHintState, error) {
	var h domain.UserTaskHintState
	err := r.db.QueryRow(ctx, `
		SELECT user_id::text, task_id::text, hint_count, COALESCE(last_hint_id::text, ''), COALESCE(last_hint_type, ''),
		       COALESCE(last_hint_attempt_number, 0), last_hint_at, updated_at
		FROM user_task_hint_state
		WHERE user_id = $1 AND task_id = $2`, userID, taskID).Scan(&h.UserID, &h.TaskID, &h.HintCount, &h.LastHintID, &h.LastHintType, &h.LastHintAttemptNumber, &h.LastHintAt, &h.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *Repository) UpsertHintState(ctx context.Context, h domain.UserTaskHintState) error {
	if h.UserID == "" || h.TaskID == "" || h.LastHintID == "" {
		return domain.ErrInvalidInput
	}
	if h.LastHintAt.IsZero() {
		h.LastHintAt = time.Now().UTC()
	}
	if h.UpdatedAt.IsZero() {
		h.UpdatedAt = h.LastHintAt
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_task_hint_state (user_id, task_id, hint_count, last_hint_id, last_hint_type, last_hint_attempt_number, last_hint_at, updated_at)
		VALUES ($1, $2, 1, $3, NULLIF($4, ''), NULLIF($5, 0), $6, $7)
		ON CONFLICT (user_id, task_id) DO UPDATE
		SET hint_count = user_task_hint_state.hint_count + 1,
		    last_hint_id = EXCLUDED.last_hint_id,
		    last_hint_type = EXCLUDED.last_hint_type,
		    last_hint_attempt_number = EXCLUDED.last_hint_attempt_number,
		    last_hint_at = EXCLUDED.last_hint_at,
		    updated_at = EXCLUDED.updated_at`, h.UserID, h.TaskID, h.LastHintID, h.LastHintType, h.LastHintAttemptNumber, h.LastHintAt, h.UpdatedAt)
	return err
}
