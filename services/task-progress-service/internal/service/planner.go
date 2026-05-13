package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"task-progress-service/internal/domain"
	"task-progress-service/internal/repository"
)

type PlannerConfig struct {
	DefaultGraphCode          string
	MasteryThreshold          float64
	PrerequisiteThreshold     float64
	RecallThreshold           float64
	RecommendationWaitTimeout time.Duration
}

type Planner struct {
	repo       *repository.Repository
	ebbinghaus *Ebbinghaus
	cfg        PlannerConfig
	publisher  EventPublisher
	log        *slog.Logger
}

func NewPlanner(repo *repository.Repository, ebbinghaus *Ebbinghaus, cfg PlannerConfig, publisher EventPublisher, log *slog.Logger) *Planner {
	return &Planner{repo: repo, ebbinghaus: ebbinghaus, cfg: cfg, publisher: publisher, log: log}
}

func (p *Planner) NextTask(ctx context.Context, userID string) (domain.NextTaskRecommendation, error) {
	if userID == "" {
		return domain.NextTaskRecommendation{}, domain.ErrInvalidInput
	}
	now := time.Now().UTC()
	analysisState, err := p.repo.GetActiveAnalysisState(ctx, userID, now)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	if analysisState != nil {
		retryAfter := 5
		if analysisState.WaitUntil != nil {
			remaining := int(time.Until(*analysisState.WaitUntil).Seconds())
			if remaining > 0 && remaining < retryAfter {
				retryAfter = remaining
			}
		}
		return domain.NextTaskRecommendation{}, &domain.AnalysisPendingError{State: *analysisState, RetryAfterSeconds: retryAfter}
	}
	profile, err := p.repo.GetOrCreateUserProfile(ctx, userID, p.cfg.DefaultGraphCode)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	graphSkills, err := p.repo.ListEffectiveGraphSkills(ctx, userID, profile.GraphID)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	deps, err := p.repo.ListEffectiveDependencies(ctx, userID, profile.GraphID)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	tasks, err := p.repo.ListTasks(ctx)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	userSkills, err := p.repo.GetUserSkillsMap(ctx, userID)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	statusByTask, err := p.repo.GetSolvedTaskIDs(ctx, userID)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	recommendations, err := p.repo.ListActiveRecommendations(ctx, userID, profile.GraphID, now)
	if err != nil {
		return domain.NextTaskRecommendation{}, err
	}
	recommendationBySkill := make(map[string]domain.UserSkillRecommendation, len(recommendations))
	for _, rec := range recommendations {
		recommendationBySkill[rec.SkillID] = rec
	}

	graphSkillMap := make(map[string]domain.GraphSkill, len(graphSkills))
	for _, gs := range graphSkills {
		graphSkillMap[gs.SkillID] = gs
	}
	depsBySkill := make(map[string][]domain.SkillDependency)
	for _, dep := range deps {
		depsBySkill[dep.SkillID] = append(depsBySkill[dep.SkillID], dep)
	}

	availableSkills := make(map[string]bool)
	for _, gs := range graphSkills {
		if p.dependenciesSatisfied(gs.SkillID, depsBySkill, graphSkillMap, userSkills, now) {
			availableSkills[gs.SkillID] = true
		}
	}

	type scoredTask struct {
		task   domain.Task
		score  float64
		reason string
		repeat bool
	}
	var newCandidates []scoredTask
	var recallCandidates []scoredTask
	var fallbackSolvedCandidates []scoredTask

	for _, task := range tasks {
		if len(task.Skills) == 0 {
			continue
		}
		covered := false
		blocked := false
		var score float64
		var weakestCode string
		var weakestValue float64 = math.MaxFloat64
		var maxForgetting float64

		for _, ts := range task.Skills {
			gs, inGraph := graphSkillMap[ts.SkillID]
			if !inGraph {
				continue
			}
			if !availableSkills[ts.SkillID] {
				blocked = true
				continue
			}
			covered = true
			userSkill, ok := userSkills[ts.SkillID]
			mastery := 0.0
			updatedAt := time.Time{}
			if ok {
				mastery = userSkill.MasteryScore
				updatedAt = userSkill.UpdatedAt
			}
			effective := 0.0
			forgetting := 1.0
			if !updatedAt.IsZero() {
				effective = p.ebbinghaus.EffectiveMastery(now, updatedAt, mastery)
				forgetting = p.ebbinghaus.ForgettingPriority(now, updatedAt, mastery)
			}
			threshold := graphSkillThreshold(gs, p.cfg.MasteryThreshold)
			weakness := clamp((threshold-effective)/math.Max(threshold, 0.01), 0, 1)
			if effective < weakestValue {
				weakestValue = effective
				weakestCode = ts.SkillCode
			}
			if forgetting > maxForgetting {
				maxForgetting = forgetting
			}
			score += ts.Weight * (weakness*0.45 + forgetting*0.30 + gs.PriorityWeight*0.15)
			if rec, ok := recommendationBySkill[ts.SkillID]; ok {
				score += ts.Weight * rec.Priority * 0.45
			}
		}
		if !covered || blocked {
			continue
		}

		status, hasStatus := statusByTask[task.ID]
		alreadySolved := hasStatus && status.Status == domain.TaskStatusSolved
		diffScore := difficultyMatchScore(task.Difficulty, weakestValue)
		score += diffScore * 0.20

		struggleScore := 0.0
		if hasStatus && status.AttemptsCount > 1 {
			struggleScore = clamp(float64(status.AttemptsCount-1)/4.0, 0, 1)
		}

		weakestThreshold := p.cfg.MasteryThreshold
		if weakestGS, ok := graphSkillByCode(graphSkillMap, weakestCode); ok {
			weakestThreshold = graphSkillThreshold(weakestGS, p.cfg.MasteryThreshold)
		}
		reason := fmt.Sprintf("подходит для навыка %s: effective_mastery=%.2f, mastery_threshold=%.2f, forgetting_priority=%.2f", weakestCode, weakestValue, weakestThreshold, maxForgetting)
		if weakestGS, ok := graphSkillByCode(graphSkillMap, weakestCode); ok {
			if rec, ok := recommendationBySkill[weakestGS.SkillID]; ok {
				reason = fmt.Sprintf("%s; приоритет LLM/analytics=%.2f (%s)", reason, rec.Priority, rec.Reason)
			}
		}
		if alreadySolved {
			repeatScore := score + maxForgetting*0.25 + struggleScore*0.35
			repeatReason := reason
			if struggleScore > 0 {
				repeatReason = fmt.Sprintf("%s; задача ранее потребовала %d попыток", repeatReason, status.AttemptsCount)
			}
			scored := scoredTask{task: task, score: repeatScore, reason: repeatReason + "; повторение для закрепления результата", repeat: true}
			fallbackSolvedCandidates = append(fallbackSolvedCandidates, scored)
			if maxForgetting >= p.cfg.RecallThreshold {
				recallCandidates = append(recallCandidates, scored)
			}
			continue
		}

		if hasStatus && status.Status == domain.TaskStatusInProgress {
			score += struggleScore * 0.25
		}
		score += 0.20
		newCandidates = append(newCandidates, scoredTask{task: task, score: score, reason: reason, repeat: false})
	}

	candidates := newCandidates
	candidateSource := "new"
	if len(candidates) == 0 && len(recallCandidates) > 0 {
		candidates = recallCandidates
		candidateSource = "recall"
	}
	if len(candidates) == 0 && len(fallbackSolvedCandidates) > 0 {
		candidates = fallbackSolvedCandidates
		candidateSource = "fallback_solved"
	}
	if len(candidates) == 0 {
		return domain.NextTaskRecommendation{}, domain.ErrNotFound
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	best := candidates[0]

	eventID, _ := domain.NewUUID()
	event := domain.TaskRecommendedEvent{
		EventID:            eventID,
		EventVersion:       1,
		EventType:          domain.EventTypeTaskRecommended,
		UserID:             userID,
		TaskID:             best.task.ID,
		GraphID:            profile.GraphID,
		GraphCode:          profile.GraphCode,
		ProfessionalTrack:  profile.ProfessionalTrack,
		Score:              best.score,
		Reason:             best.reason,
		RepeatMode:         best.repeat,
		CandidateSource:    candidateSource,
		NewCandidates:      len(newCandidates),
		RecallCandidates:   len(recallCandidates),
		FallbackCandidates: len(fallbackSolvedCandidates),
		RecommendedSkills:  recommendations,
		CreatedAt:          now,
	}
	if err := p.publisher.PublishTaskRecommended(ctx, event); err != nil {
		p.log.Error("publish task recommended failed", "error", err, "user_id", userID, "task_id", best.task.ID)
	}

	return domain.NextTaskRecommendation{Task: best.task, Reason: best.reason, Score: best.score, GraphCode: profile.GraphCode, Track: profile.ProfessionalTrack, RepeatMode: best.repeat, RecommendedSkills: recommendations}, nil
}

func (p *Planner) dependenciesSatisfied(skillID string, depsBySkill map[string][]domain.SkillDependency, graphSkillMap map[string]domain.GraphSkill, userSkills map[string]domain.UserSkill, now time.Time) bool {
	deps := depsBySkill[skillID]
	if len(deps) == 0 {
		return true
	}
	for _, dep := range deps {
		userSkill, ok := userSkills[dep.DependsOnSkillID]
		if !ok {
			return false
		}
		effective := p.ebbinghaus.EffectiveMastery(now, userSkill.UpdatedAt, userSkill.MasteryScore)
		threshold := p.cfg.PrerequisiteThreshold * dep.Strength
		if prereqGS, ok := graphSkillMap[dep.DependsOnSkillID]; ok {
			threshold = graphSkillThreshold(prereqGS, p.cfg.PrerequisiteThreshold) * dep.Strength
		}
		if dep.RequiredMastery != nil {
			threshold = *dep.RequiredMastery
		}
		threshold = clamp(threshold, 0, 1)
		if effective < threshold {
			return false
		}
	}
	return true
}

func graphSkillThreshold(gs domain.GraphSkill, fallback float64) float64 {
	if gs.MasteryThreshold > 0 && gs.MasteryThreshold <= 1 {
		return gs.MasteryThreshold
	}
	return fallback
}

func graphSkillByCode(items map[string]domain.GraphSkill, code string) (domain.GraphSkill, bool) {
	for _, item := range items {
		if item.SkillCode == code {
			return item, true
		}
	}
	return domain.GraphSkill{}, false
}

func difficultyMatchScore(difficulty string, effectiveMastery float64) float64 {
	if effectiveMastery < 0.45 {
		if difficulty == domain.DifficultyEasy {
			return 1.0
		}
		if difficulty == domain.DifficultyMedium {
			return 0.55
		}
		return 0.25
	}
	if effectiveMastery < 0.75 {
		if difficulty == domain.DifficultyMedium {
			return 1.0
		}
		return 0.65
	}
	if difficulty == domain.DifficultyHard {
		return 1.0
	}
	if difficulty == domain.DifficultyMedium {
		return 0.75
	}
	return 0.45
}
