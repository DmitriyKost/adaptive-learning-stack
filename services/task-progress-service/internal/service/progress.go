package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"task-progress-service/internal/domain"
	"task-progress-service/internal/repository"
)

type ProgressUsecase struct {
	repo       *repository.Repository
	publisher  EventPublisher
	hints      HintGeneratorClient
	ebbinghaus *Ebbinghaus
	cfg        PlannerConfig
	log        *slog.Logger
}

type SubmitAttemptInput struct {
	SourceEventID    string
	AttemptID        string
	UserID           string
	TaskID           string
	SubmittedSQL     string
	ExecutionSuccess bool
	IsCorrect        *bool
	Columns          []string
	Rows             [][]any
	ExpectedColumns  []string
	ExpectedRows     [][]any
	ExecutionTimeMS  int64
	RowCount         int
	ErrorType        string
	ErrorMessage     string
	QueryHash        string
	HintRequested    bool
	HintUsed         bool
	HintID           string
	HintType         string
	HintsCount       int
	CreatedAt        time.Time
}

type SubmitAttemptResult struct {
	Attempt      domain.TaskAttempt                `json:"attempt"`
	SkillUpdates []domain.LearnerModelUpdatedEvent `json:"skill_updates"`
}

func NewProgressUsecase(repo *repository.Repository, publisher EventPublisher, hints HintGeneratorClient, ebbinghaus *Ebbinghaus, cfg PlannerConfig, log *slog.Logger) *ProgressUsecase {
	return &ProgressUsecase{repo: repo, publisher: publisher, hints: hints, ebbinghaus: ebbinghaus, cfg: cfg, log: log}
}

func (u *ProgressUsecase) ListTasks(ctx context.Context) ([]domain.Task, error) {
	return u.repo.ListTasks(ctx)
}

func (u *ProgressUsecase) GetTask(ctx context.Context, taskID string) (domain.Task, error) {
	if taskID == "" {
		return domain.Task{}, domain.ErrInvalidInput
	}
	return u.repo.GetTaskByID(ctx, taskID)
}

func (u *ProgressUsecase) SubmitAttempt(ctx context.Context, input SubmitAttemptInput) (SubmitAttemptResult, error) {
	if input.UserID == "" || input.TaskID == "" {
		return SubmitAttemptResult{}, domain.ErrInvalidInput
	}
	task, err := u.repo.GetTaskByID(ctx, input.TaskID)
	if err != nil {
		return SubmitAttemptResult{}, err
	}

	isCorrect := false
	if input.ExecutionSuccess {
		if input.IsCorrect != nil {
			isCorrect = *input.IsCorrect
		} else if len(input.ExpectedColumns) > 0 || len(input.ExpectedRows) > 0 {
			isCorrect = compareTabularResults(input.Columns, input.Rows, input.ExpectedColumns, input.ExpectedRows, task.OrderSensitive)
		}
	}

	attemptID := input.AttemptID
	if attemptID == "" {
		var err error
		attemptID, err = domain.NewUUID()
		if err != nil {
			return SubmitAttemptResult{}, err
		}
	}
	now := input.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	attempt := domain.TaskAttempt{
		ID:               attemptID,
		UserID:           input.UserID,
		TaskID:           input.TaskID,
		SubmittedSQL:     input.SubmittedSQL,
		ExecutionSuccess: input.ExecutionSuccess,
		IsCorrect:        isCorrect,
		ExecutionTimeMS:  input.ExecutionTimeMS,
		RowCount:         input.RowCount,
		ErrorType:        input.ErrorType,
		ErrorMessage:     input.ErrorMessage,
		QueryHash:        input.QueryHash,
		CreatedAt:        now,
	}

	deltas := make([]repository.SkillDelta, 0, len(task.Skills))
	for _, skill := range task.Skills {
		delta := calculateSkillDelta(isCorrect, task.Difficulty, skill.Weight)
		deltas = append(deltas, repository.SkillDelta{SkillID: skill.SkillID, SkillCode: skill.SkillCode, Weight: skill.Weight, Delta: delta})
	}

	attemptNumber, updates, analysisState, err := u.repo.CreateAttemptAndUpdateModel(ctx, attempt, task, deltas, input.SourceEventID, u.cfg.RecommendationWaitTimeout)
	if errors.Is(err, domain.ErrDuplicateEvent) {
		u.log.Info("playground event already processed", "event_id", input.SourceEventID, "user_id", input.UserID, "task_id", input.TaskID)
		return SubmitAttemptResult{}, nil
	}
	if err != nil {
		return SubmitAttemptResult{}, err
	}

	affected := make([]domain.AffectedSkill, 0, len(deltas))
	for _, d := range deltas {
		affected = append(affected, domain.AffectedSkill{SkillID: d.SkillID, SkillCode: d.SkillCode, Weight: d.Weight, Delta: d.Delta})
	}

	actualResult := &domain.QueryResult{Columns: input.Columns, Rows: input.Rows, RowCount: input.RowCount, ExecutionTimeMS: input.ExecutionTimeMS}
	if len(input.Columns) == 0 && len(input.Rows) == 0 && input.RowCount == 0 && input.ExecutionTimeMS == 0 {
		actualResult = nil
	}
	expectedResult := &domain.QueryResult{Columns: input.ExpectedColumns, Rows: input.ExpectedRows}
	if len(input.ExpectedColumns) == 0 && len(input.ExpectedRows) == 0 {
		expectedResult = nil
	}
	hint := domain.HintContext{}
	if hintState, err := u.repo.GetHintState(ctx, input.UserID, input.TaskID); err != nil {
		u.log.Error("load hint state failed", "error", err, "user_id", input.UserID, "task_id", input.TaskID)
	} else if hintState != nil && hintState.HintCount > 0 {
		hint = domain.HintContext{HintRequested: true, HintUsed: true, HintID: hintState.LastHintID, HintType: hintState.LastHintType, HintsCount: hintState.HintCount}
	}
	taskCtx := domain.TaskContext{TaskID: task.ID, Title: task.Title, Description: task.Description, Difficulty: task.Difficulty, DatasetID: task.DatasetID, ReferenceSQL: task.ReferenceSQL, Skills: task.Skills}
	execCtx := domain.AttemptExecutionContext{AttemptID: attemptID, AttemptNumber: attemptNumber, ExecutionSuccess: input.ExecutionSuccess, IsCorrect: isCorrect, SubmittedSQL: input.SubmittedSQL, QueryHash: input.QueryHash, ExecutionTimeMS: input.ExecutionTimeMS, RowCount: input.RowCount, ErrorType: input.ErrorType, ErrorMessage: input.ErrorMessage, ActualResult: actualResult, ExpectedResult: expectedResult}

	taskEventID, _ := domain.NewUUID()
	if err := u.publisher.PublishTaskChecked(ctx, domain.TaskCheckedEvent{
		EventID:          taskEventID,
		EventVersion:     1,
		EventType:        domain.EventTypeTaskChecked,
		UserID:           input.UserID,
		TaskID:           input.TaskID,
		TaskTitle:        task.Title,
		TaskDescription:  task.Description,
		Difficulty:       task.Difficulty,
		DatasetID:        task.DatasetID,
		ReferenceSQL:     task.ReferenceSQL,
		AttemptID:        attemptID,
		AttemptNumber:    attemptNumber,
		ExecutionSuccess: input.ExecutionSuccess,
		IsCorrect:        isCorrect,
		ExecutionTimeMS:  input.ExecutionTimeMS,
		RowCount:         input.RowCount,
		ErrorType:        input.ErrorType,
		ErrorMessage:     input.ErrorMessage,
		QueryHash:        input.QueryHash,
		SubmittedSQL:     input.SubmittedSQL,
		ActualResult:     actualResult,
		ExpectedResult:   expectedResult,
		Hint:             hint,
		Task:             taskCtx,
		Execution:        execCtx,
		AffectedSkills:   affected,
		SourceEventID:    input.SourceEventID,
		CreatedAt:        now,
	}); err != nil {
		u.log.Error("publish task checked failed", "error", err, "user_id", input.UserID, "task_id", input.TaskID)
	}

	if isCorrect {
		var graphState *domain.GraphStateSnapshot
		if snapshot, err := u.BuildGraphStateSnapshot(ctx, input.UserID, now); err != nil {
			u.log.Error("build graph state snapshot failed", "error", err, "user_id", input.UserID, "task_id", input.TaskID)
		} else {
			graphState = &snapshot
		}

		completedEventID, _ := domain.NewUUID()
		if err := u.publisher.PublishTaskCompleted(ctx, domain.TaskCompletedEvent{
			EventID:         completedEventID,
			EventVersion:    1,
			EventType:       domain.EventTypeTaskCompleted,
			UserID:          input.UserID,
			TaskID:          input.TaskID,
			TaskTitle:       task.Title,
			TaskDescription: task.Description,
			Difficulty:      task.Difficulty,
			DatasetID:       task.DatasetID,
			ReferenceSQL:    task.ReferenceSQL,
			AttemptID:       attemptID,
			AttemptNumber:   attemptNumber,
			ExecutionTimeMS: input.ExecutionTimeMS,
			QueryHash:       input.QueryHash,
			SubmittedSQL:    input.SubmittedSQL,
			ActualResult:    actualResult,
			ExpectedResult:  expectedResult,
			Hint:            hint,
			Task:            taskCtx,
			Execution:       execCtx,
			AffectedSkills:  affected,
			GraphState:      graphState,
			AnalysisState:   analysisState,
			SourceEventID:   input.SourceEventID,
			CreatedAt:       now,
		}); err != nil {
			u.log.Error("publish task completed failed", "error", err, "user_id", input.UserID, "task_id", input.TaskID)
		}
	}

	skillEvents := make([]domain.LearnerModelUpdatedEvent, 0, len(updates))
	for _, update := range updates {
		eventID, _ := domain.NewUUID()
		event := domain.LearnerModelUpdatedEvent{
			EventID:         eventID,
			EventVersion:    1,
			EventType:       domain.EventTypeLearnerModelUpdated,
			UserID:          input.UserID,
			SkillID:         update.SkillID,
			SkillCode:       update.SkillCode,
			OldMasteryScore: update.OldMastery,
			NewMasteryScore: update.NewMastery,
			OldConfidence:   update.OldConfidence,
			NewConfidence:   update.NewConfidence,
			Source:          "task-progress-service",
			Reason:          fmt.Sprintf("task %s attempt", input.TaskID),
			CreatedAt:       now,
		}
		skillEvents = append(skillEvents, event)
		if err := u.publisher.PublishLearnerModelUpdated(ctx, event); err != nil {
			u.log.Error("publish learner model updated failed", "error", err, "user_id", input.UserID, "skill_id", update.SkillID)
		}
	}

	return SubmitAttemptResult{Attempt: attempt, SkillUpdates: skillEvents}, nil
}

func (u *ProgressUsecase) GenerateHint(ctx context.Context, userID, taskID string) (domain.HintGenerateResponse, error) {
	if userID == "" || taskID == "" {
		return domain.HintGenerateResponse{}, domain.ErrInvalidInput
	}
	status, err := u.repo.GetUserTaskStatus(ctx, userID, taskID)
	if err != nil {
		return domain.HintGenerateResponse{}, err
	}
	if status.AttemptsCount < 1 {
		return domain.HintGenerateResponse{}, domain.ErrHintUnavailable
	}
	if u.hints == nil {
		return domain.HintGenerateResponse{}, domain.ErrHintGenerationFailed
	}
	requestID, _ := domain.NewUUID()
	resp, err := u.hints.GenerateHint(ctx, domain.HintGenerateRequest{
		UserID:               userID,
		TaskID:               taskID,
		CurrentAttemptNumber: status.AttemptsCount + 1,
		RequestID:            requestID,
	})
	if err != nil {
		return domain.HintGenerateResponse{}, domain.ErrHintGenerationFailed
	}
	if resp.CreatedAt.IsZero() {
		resp.CreatedAt = time.Now().UTC()
	}
	if resp.UserID == "" {
		resp.UserID = userID
	}
	if resp.TaskID == "" {
		resp.TaskID = taskID
	}
	if resp.AttemptNumber == 0 {
		resp.AttemptNumber = status.AttemptsCount + 1
	}
	if err := u.repo.UpsertHintState(ctx, domain.UserTaskHintState{
		UserID:                userID,
		TaskID:                taskID,
		HintCount:             1,
		LastHintID:            resp.HintID,
		LastHintType:          resp.HintType,
		LastHintAttemptNumber: resp.AttemptNumber,
		LastHintAt:            resp.CreatedAt,
		UpdatedAt:             resp.CreatedAt,
	}); err != nil {
		return domain.HintGenerateResponse{}, err
	}
	return resp, nil
}

func (u *ProgressUsecase) GetProgressSummary(ctx context.Context, userID string) (domain.ProgressSummary, error) {
	if userID == "" {
		return domain.ProgressSummary{}, domain.ErrInvalidInput
	}
	profile, err := u.repo.GetOrCreateUserProfile(ctx, userID, u.cfg.DefaultGraphCode)
	if err != nil {
		return domain.ProgressSummary{}, err
	}
	graphSkills, err := u.repo.ListEffectiveGraphSkills(ctx, userID, profile.GraphID)
	if err != nil {
		return domain.ProgressSummary{}, err
	}
	graphSkillMap := make(map[string]domain.GraphSkill, len(graphSkills))
	for _, gs := range graphSkills {
		graphSkillMap[gs.SkillID] = gs
	}
	skills, err := u.repo.GetUserSkills(ctx, userID)
	if err != nil {
		return domain.ProgressSummary{}, err
	}
	completed, attempts, err := u.repo.CountUserAttempts(ctx, userID)
	if err != nil {
		return domain.ProgressSummary{}, err
	}

	now := time.Now().UTC()
	views := make([]domain.UserSkillView, 0, len(skills))
	weakSkills := make([]domain.UserSkillView, 0)
	recallSkills := make([]domain.UserSkillView, 0)
	var avgMastery, avgEffective float64
	for _, skill := range skills {
		updatedAt := skill.UpdatedAt
		retention := u.ebbinghaus.Retention(now, updatedAt, skill.MasteryScore)
		effective := skill.MasteryScore * retention
		threshold := u.cfg.MasteryThreshold
		priority := 1.0
		required := false
		if gs, ok := graphSkillMap[skill.SkillID]; ok {
			threshold = graphSkillThreshold(gs, u.cfg.MasteryThreshold)
			priority = gs.PriorityWeight
			required = gs.IsRequired
		}
		view := domain.UserSkillView{
			UserSkill:           skill,
			Retention:           retention,
			ForgettingPriority:  1 - retention,
			EffectiveMastery:    effective,
			MasteryThreshold:    threshold,
			GraphPriorityWeight: priority,
			GraphRequired:       required,
		}
		avgMastery += skill.MasteryScore
		avgEffective += effective
		views = append(views, view)
		if effective < threshold {
			weakSkills = append(weakSkills, view)
		}
		if view.ForgettingPriority >= u.cfg.RecallThreshold {
			recallSkills = append(recallSkills, view)
		}
	}
	if len(skills) > 0 {
		avgMastery /= float64(len(skills))
		avgEffective /= float64(len(skills))
	}
	recommendations, err := u.repo.ListActiveRecommendations(ctx, userID, profile.GraphID, now)
	if err != nil {
		return domain.ProgressSummary{}, err
	}
	analysisState, err := u.repo.GetActiveAnalysisState(ctx, userID, now)
	if err != nil {
		return domain.ProgressSummary{}, err
	}

	return domain.ProgressSummary{
		UserID:                userID,
		Profile:               profile,
		CompletedTasks:        completed,
		TotalAttempts:         attempts,
		AverageMasteryScore:   avgMastery,
		AverageEffectiveScore: avgEffective,
		Skills:                views,
		WeakSkills:            weakSkills,
		RecallSkills:          recallSkills,
		RecommendedSkills:     recommendations,
		AnalysisState:         analysisState,
	}, nil
}

func (u *ProgressUsecase) BuildGraphStateSnapshot(ctx context.Context, userID string, now time.Time) (domain.GraphStateSnapshot, error) {
	if userID == "" {
		return domain.GraphStateSnapshot{}, domain.ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	profile, err := u.repo.GetOrCreateUserProfile(ctx, userID, u.cfg.DefaultGraphCode)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}
	graph, err := u.repo.GetGraphByID(ctx, profile.GraphID)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}
	graphSkills, err := u.repo.ListEffectiveGraphSkills(ctx, userID, profile.GraphID)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}
	dependencies, err := u.repo.ListEffectiveDependencies(ctx, userID, profile.GraphID)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}
	recommendations, err := u.repo.ListActiveRecommendations(ctx, userID, profile.GraphID, now)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}
	userSkills, err := u.repo.GetUserSkillsMap(ctx, userID)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}
	completed, attempts, err := u.repo.CountUserGraphAttempts(ctx, userID, profile.GraphID)
	if err != nil {
		return domain.GraphStateSnapshot{}, err
	}

	states := make([]domain.GraphSkillState, 0, len(graphSkills))
	var avgMastery, avgEffective, avgGap float64
	var masteredCount, weakCount, recallCount int
	for _, gs := range graphSkills {
		threshold := graphSkillThreshold(gs, u.cfg.MasteryThreshold)
		var mastery, confidence float64
		var attemptsCount, successCount int
		var lastUsedAt *time.Time
		var updatedAt *time.Time
		retention := 1.0
		forgetting := 0.0
		if us, ok := userSkills[gs.SkillID]; ok {
			mastery = us.MasteryScore
			confidence = us.Confidence
			attemptsCount = us.AttemptsCount
			successCount = us.SuccessCount
			lastUsedAt = us.LastUsedAt
			updated := us.UpdatedAt
			updatedAt = &updated
			retention = u.ebbinghaus.Retention(now, us.UpdatedAt, us.MasteryScore)
			forgetting = 1 - retention
		}
		effective := mastery * retention
		gap := clamp(threshold-effective, 0, 1)
		mastered := effective >= threshold
		if mastered {
			masteredCount++
		} else {
			weakCount++
		}
		if forgetting >= u.cfg.RecallThreshold {
			recallCount++
		}
		avgMastery += mastery
		avgEffective += effective
		avgGap += gap
		states = append(states, domain.GraphSkillState{
			SkillID:            gs.SkillID,
			SkillCode:          gs.SkillCode,
			SkillName:          gs.SkillName,
			Position:           gs.Position,
			IsRequired:         gs.IsRequired,
			PriorityWeight:     gs.PriorityWeight,
			MasteryThreshold:   threshold,
			MasteryScore:       mastery,
			Confidence:         confidence,
			AttemptsCount:      attemptsCount,
			SuccessCount:       successCount,
			Retention:          retention,
			ForgettingPriority: forgetting,
			EffectiveMastery:   effective,
			ThresholdGap:       gap,
			Mastered:           mastered,
			LastUsedAt:         lastUsedAt,
			UpdatedAt:          updatedAt,
		})
	}
	if len(states) > 0 {
		denom := float64(len(states))
		avgMastery /= denom
		avgEffective /= denom
		avgGap /= denom
	}
	depStates := make([]domain.GraphDependencyState, 0, len(dependencies))
	for _, dep := range dependencies {
		depStates = append(depStates, domain.GraphDependencyState{
			GraphID:            dep.GraphID,
			SkillID:            dep.SkillID,
			SkillCode:          dep.SkillCode,
			DependsOnSkillID:   dep.DependsOnSkillID,
			DependsOnSkillCode: dep.DependsOnSkillCode,
			Strength:           dep.Strength,
			RequiredMastery:    dep.RequiredMastery,
		})
	}
	return domain.GraphStateSnapshot{
		UserID:                userID,
		Profile:               profile,
		Graph:                 graph,
		CompletedTasks:        completed,
		TotalAttempts:         attempts,
		AverageMasteryScore:   avgMastery,
		AverageEffectiveScore: avgEffective,
		AverageThresholdGap:   avgGap,
		MasteredSkillsCount:   masteredCount,
		WeakSkillsCount:       weakCount,
		RecallSkillsCount:     recallCount,
		Skills:                states,
		Dependencies:          depStates,
		RecommendedSkills:     recommendations,
		CreatedAt:             now,
	}, nil
}

func (u *ProgressUsecase) SetUserGraph(ctx context.Context, userID, graphCode, graphID, track, reason string) (domain.UserLearningProfile, error) {
	if userID == "" {
		return domain.UserLearningProfile{}, domain.ErrInvalidInput
	}
	if graphID == "" {
		if graphCode == "" {
			return domain.UserLearningProfile{}, domain.ErrInvalidInput
		}
		graph, err := u.repo.GetGraphByCode(ctx, graphCode)
		if err != nil {
			return domain.UserLearningProfile{}, err
		}
		graphID = graph.ID
	}
	if track == "" {
		track = graphCode
	}
	return u.repo.SetUserProfile(ctx, userID, graphID, track, reason)
}

func calculateSkillDelta(isCorrect bool, difficulty string, weight float64) float64 {
	base := -0.05
	if isCorrect {
		base = 0.10
	}
	difficultyFactor := 1.0
	switch difficulty {
	case domain.DifficultyEasy:
		difficultyFactor = 0.8
	case domain.DifficultyHard:
		difficultyFactor = 1.2
	}
	return base * difficultyFactor * weight
}
