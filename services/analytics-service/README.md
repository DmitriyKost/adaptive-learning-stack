# analytics-service

Сервис аналитики и LLM/intelligence integration.

Он:

- читает Kafka events от task-progress-service;
- пишет raw и структурированные события в ClickHouse;
- собирает context для внешнего intelligence service;
- вызывает /v1/assessments/evaluate;
- сохраняет request/response analysis runs;
- публикует analytics.skill_assessment.updated;
- генерирует подсказки через /v1/hints/generate.

Локальный mock-intelligence режим не является основным контрактом. Для полного E2E нужен внешний intelligence service.

## External intelligence service

Ожидаемые endpoints:

    GET  /ready
    POST /v1/assessments/evaluate
    POST /v1/hints/generate

Основные env:

    INTELLIGENCE_BASE_URL=http://intelligence:8080
    INTELLIGENCE_API_KEY=
    INTELLIGENCE_READY_PATH=/ready
    INTELLIGENCE_EVALUATE_PATH=/v1/assessments/evaluate
    INTELLIGENCE_HINT_PATH=/v1/hints/generate
    INTELLIGENCE_HTTP_TIMEOUT=600s
    INTELLIGENCE_INCLUDE_CAREER=false
    HINT_GENERATION_TIMEOUT=600s

Если INTELLIGENCE_BASE_URL не задан, сервис не должен считаться ready.

## Kafka input

    task.checked
    task.completed
    task.recommended
    learner_model.updated

Сервис коммитит offset только после успешной обработки. Для task.completed обработка включает вызов external intelligence и публикацию analytics.skill_assessment.updated.

## Kafka output

    analytics.skill_assessment.updated
    analytics.career_recommendation.created
    analytics.hint.generated

Основное событие:

    {
      "event_type": "analytics.skill_assessment.updated",
      "user_id": "...",
      "source": "analytics-service/intelligence",
      "graph_code": "core_sql",
      "professional_track": "core",
      "source_task_id": "...",
      "source_attempt_id": "...",
      "observed_until": "...",
      "assessment_mode": "merge",
      "analysis": {
        "analysis_run_id": "...",
        "model_version": "sql-intelligence-v1",
        "prompt_version": "sql-mastery-context-v1"
      },
      "user_graph_overlay": {
        "graph_code": "core_sql",
        "professional_track": "core",
        "skills": []
      },
      "recommended_skills": [],
      "skill_scores": []
    }

recommended_skills отправляется как snapshot для пользователя и графа.

## ClickHouse

Сервис пишет:

- raw_events;
- task_attempt_logs;
- task_completion_logs;
- task_recommendation_logs;
- learner_model_update_logs;
- llm_analysis_runs;
- skill_assessment_logs;
- skill_recommendation_logs;
- hint_events.

request_context в llm_analysis_runs содержит internal task context, включая reference SQL и expected/user results. Это не public API.

## Reference graphs

analytics-service не изменяет global reference graphs. Все user-specific изменения идут через user_graph_overlay, который применяет task-progress-service.

## API

    GET  /health
    GET  /ready
    POST /internal/hints/generate

GET /health проверяет HTTP-процесс.

GET /ready проверяет ClickHouse и внешний intelligence service.

## Hint generation

Internal endpoint:

    POST /internal/hints/generate

Request:

    {
      "user_id": "...",
      "task_id": "...",
      "current_attempt_number": 2,
      "request_id": "..."
    }

Сервис загружает историю попыток пользователя по задаче, task metadata, reference SQL, ошибки, предыдущие подсказки и вызывает внешний intelligence service.

Успешные подсказки сохраняются в hint_events и публикуются как analytics.hint.generated.

Если подсказка не сгенерирована, событие показа подсказки не сохраняется.

## Local commands

    go test ./...
    go run ./main.go

## E2E path

    playground-service
    -> playground.execution.completed
    -> task-progress-service
    -> task.completed
    -> analytics-service
    -> external intelligence service
    -> analytics.skill_assessment.updated
    -> task-progress-service
    -> learner model update
    -> persisted next task

Proof queries обычно смотрят:

    ClickHouse:
      llm_analysis_runs
      skill_assessment_logs
      learner_model_update_logs

    Postgres task-progress:
      user_skills
      user_skill_assessment_versions
      user_next_task_recommendations
