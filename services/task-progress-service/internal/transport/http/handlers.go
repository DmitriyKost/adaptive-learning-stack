package httptransport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"task-progress-service/internal/domain"
	"task-progress-service/internal/repository"
	"task-progress-service/internal/service"
)

type Server struct {
	progress            *service.ProgressUsecase
	planner             *service.Planner
	repo                *repository.Repository
	tokens              *TokenValidator
	allowGatewayHeaders bool
	defaultGraphCode    string
	log                 *slog.Logger
}

func NewServer(progress *service.ProgressUsecase, planner *service.Planner, repo *repository.Repository, tokens *TokenValidator, allowGatewayHeaders bool, defaultGraphCode string, log *slog.Logger) *Server {
	return &Server{progress: progress, planner: planner, repo: repo, tokens: tokens, allowGatewayHeaders: allowGatewayHeaders, defaultGraphCode: defaultGraphCode, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/ready", s.ready)

	protected := http.NewServeMux()
	protected.HandleFunc("/tasks", s.tasks)
	protected.HandleFunc("/tasks/next", s.nextTask)
	protected.HandleFunc("/tasks/", s.taskByID)
	protected.HandleFunc("/progress/me", s.progressMe)
	protected.HandleFunc("/progress/skills", s.progressSkills)
	protected.HandleFunc("/users/me/learning-graph", s.userLearningGraph)
	protected.HandleFunc("/graphs", s.graphs)
	protected.HandleFunc("/graphs/", s.graphByID)
	protected.HandleFunc("/skills", s.skills)

	mux.Handle("/tasks", s.authMiddleware(protected))
	mux.Handle("/tasks/", s.authMiddleware(protected))
	mux.Handle("/progress/me", s.authMiddleware(protected))
	mux.Handle("/progress/skills", s.authMiddleware(protected))
	mux.Handle("/users/me/learning-graph", s.authMiddleware(protected))
	mux.Handle("/graphs", s.authMiddleware(protected))
	mux.Handle("/graphs/", s.authMiddleware(protected))
	mux.Handle("/skills", s.authMiddleware(protected))

	return s.recoveryMiddleware(s.loggingMiddleware(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.repo.Ping(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	tasks, err := s.progress.ListTasks(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) taskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, domain.ErrNotFound)
		return
	}

	if strings.HasSuffix(path, "/hint") {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		taskID := strings.TrimSuffix(path, "/hint")
		taskID = strings.Trim(taskID, "/")
		s.generateHint(w, r, taskID)
		return
	}

	if strings.HasSuffix(path, "/submit") {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		taskID := strings.TrimSuffix(path, "/submit")
		taskID = strings.Trim(taskID, "/")
		s.submitTask(w, r, taskID)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	task, err := s.progress.GetTask(r.Context(), path)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) nextTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	rec, err := s.planner.NextTask(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) generateHint(w http.ResponseWriter, r *http.Request, taskID string) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	resp, err := s.progress.GenerateHint(r.Context(), claims.UserID, taskID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type submitTaskRequest struct {
	SubmittedSQL     string   `json:"submitted_sql"`
	ExecutionSuccess bool     `json:"execution_success"`
	IsCorrect        *bool    `json:"is_correct,omitempty"`
	Columns          []string `json:"columns,omitempty"`
	Rows             [][]any  `json:"rows,omitempty"`
	ExpectedColumns  []string `json:"expected_columns,omitempty"`
	ExpectedRows     [][]any  `json:"expected_rows,omitempty"`
	ExecutionTimeMS  int64    `json:"execution_time_ms"`
	RowCount         int      `json:"row_count"`
	ErrorType        string   `json:"error_type,omitempty"`
	ErrorMessage     string   `json:"error_message,omitempty"`
	QueryHash        string   `json:"query_hash,omitempty"`
	HintRequested    bool     `json:"hint_requested,omitempty"`
	HintUsed         bool     `json:"hint_used,omitempty"`
	HintID           string   `json:"hint_id,omitempty"`
	HintType         string   `json:"hint_type,omitempty"`
	HintsCount       int      `json:"hints_count,omitempty"`
}

func (s *Server) submitTask(w http.ResponseWriter, r *http.Request, taskID string) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	var req submitTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domain.ErrInvalidInput)
		return
	}
	result, err := s.progress.SubmitAttempt(r.Context(), service.SubmitAttemptInput{
		UserID:           claims.UserID,
		TaskID:           taskID,
		SubmittedSQL:     req.SubmittedSQL,
		ExecutionSuccess: req.ExecutionSuccess,
		IsCorrect:        req.IsCorrect,
		Columns:          req.Columns,
		Rows:             req.Rows,
		ExpectedColumns:  req.ExpectedColumns,
		ExpectedRows:     req.ExpectedRows,
		ExecutionTimeMS:  req.ExecutionTimeMS,
		RowCount:         req.RowCount,
		ErrorType:        req.ErrorType,
		ErrorMessage:     req.ErrorMessage,
		QueryHash:        req.QueryHash,
		HintRequested:    req.HintRequested,
		HintUsed:         req.HintUsed,
		HintID:           req.HintID,
		HintType:         req.HintType,
		HintsCount:       req.HintsCount,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) progressMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	summary, err := s.progress.GetProgressSummary(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) progressSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	summary, err := s.progress.GetProgressSummary(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary.Skills)
}

type setGraphRequest struct {
	GraphID           string `json:"graph_id,omitempty"`
	GraphCode         string `json:"graph_code,omitempty"`
	ProfessionalTrack string `json:"professional_track,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

func (s *Server) userLearningGraph(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		profile, err := s.repo.GetOrCreateUserProfile(r.Context(), claims.UserID, s.defaultGraphCode)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodPut:
		var req setGraphRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.ErrInvalidInput)
			return
		}
		profile, err := s.progress.SetUserGraph(r.Context(), claims.UserID, req.GraphCode, req.GraphID, req.ProfessionalTrack, req.Reason)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, profile)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type createGraphRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

func (s *Server) graphs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		graphs, err := s.repo.ListGraphs(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, graphs)
	case http.MethodPost:
		claims, ok := claimsFromContext(r.Context())
		if !ok {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		if claims.Role != "admin" {
			writeError(w, domain.ErrForbidden)
			return
		}
		var req createGraphRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.ErrInvalidInput)
			return
		}
		active := true
		if req.IsActive != nil {
			active = *req.IsActive
		}
		graph, err := s.repo.CreateGraph(r.Context(), domain.LearningGraph{Code: req.Code, Name: req.Name, Description: req.Description, IsActive: active})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, graph)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type createSkillRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain,omitempty"`
}

func (s *Server) skills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills, err := s.repo.ListSkills(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skills)
	case http.MethodPost:
		claims, ok := claimsFromContext(r.Context())
		if !ok {
			writeError(w, domain.ErrUnauthorized)
			return
		}
		if claims.Role != "admin" {
			writeError(w, domain.ErrForbidden)
			return
		}
		var req createSkillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.ErrInvalidInput)
			return
		}
		skill, err := s.repo.CreateSkill(r.Context(), domain.Skill{Code: req.Code, Name: req.Name, Description: req.Description, Domain: req.Domain})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, skill)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type upsertGraphSkillRequest struct {
	SkillID          string  `json:"skill_id"`
	Position         int     `json:"position"`
	IsRequired       bool    `json:"is_required"`
	PriorityWeight   float64 `json:"priority_weight"`
	MasteryThreshold float64 `json:"mastery_threshold"`
}

type upsertDependencyRequest struct {
	SkillID          string   `json:"skill_id"`
	DependsOnSkillID string   `json:"depends_on_skill_id"`
	Strength         float64  `json:"strength"`
	RequiredMastery  *float64 `json:"required_mastery,omitempty"`
}

func (s *Server) graphByID(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	if claims.Role != "admin" {
		writeError(w, domain.ErrForbidden)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/graphs/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		writeError(w, domain.ErrNotFound)
		return
	}
	graphID := parts[0]
	switch parts[1] {
	case "skills":
		var req upsertGraphSkillRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.ErrInvalidInput)
			return
		}
		if req.PriorityWeight == 0 {
			req.PriorityWeight = 1
		}
		if req.MasteryThreshold == 0 {
			req.MasteryThreshold = 0.70
		}
		if err := s.repo.UpsertGraphSkill(r.Context(), graphID, req.SkillID, req.Position, req.IsRequired, req.PriorityWeight, req.MasteryThreshold); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "dependencies":
		var req upsertDependencyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.ErrInvalidInput)
			return
		}
		if req.Strength == 0 {
			req.Strength = 1
		}
		if err := s.repo.UpsertDependency(r.Context(), graphID, req.SkillID, req.DependsOnSkillID, req.Strength, req.RequiredMastery); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		writeError(w, domain.ErrNotFound)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return domain.ErrInvalidInput
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			return err
		}
		return domain.ErrInvalidInput
	}
	return nil
}
