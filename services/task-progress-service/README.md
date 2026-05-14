# task-progress-service

Сервис задач, прогресса, learner model и выбора следующей задачи.

Он:

- хранит skills, learning graphs, dependencies и SQL tasks;
- хранит server-side reference_sql;
- хранит comparison policy задачи;
- отдаёт public список задач без reference_sql;
- отдаёт internal reference context для playground-service;
- принимает playground.execution.completed;
- обновляет оперативную learner model;
- блокирует выдачу следующей задачи на время LLM analysis;
- применяет analytics.skill_assessment.updated;
- сохраняет persisted next task.

## Public API

Служебные endpoints:

    GET /health
    GET /ready

Tasks:

    GET  /tasks
    GET  /tasks/{id}
    GET  /tasks/next
    POST /tasks/{id}/hint

Progress:

    GET /progress/me
    GET /progress/skills

Learning graph:

    GET /users/me/learning-graph
    PUT /users/me/learning-graph

Skills and graphs:

    GET  /skills
    POST /skills

    GET  /graphs
    POST /graphs
    POST /graphs/{id}/skills
    POST /graphs/{id}/dependencies

Public task responses не содержат reference_sql и не содержат order_sensitive.

## Disabled legacy endpoint

Старый endpoint отключён:

    POST /tasks/{id}/submit

Ожидаемый ответ:

    HTTP/1.1 410 Gone

    {
      "error": "legacy_submit_disabled"
    }

Единственный public submit path — POST /playground/execute.

## Internal API

    GET /internal/tasks/{id}/reference

Используется playground-service.

Response:

    {
      "task_id": "00000000-0000-0000-0000-000000010003",
      "reference_sql": "SELECT id, name, salary FROM task_data.employees ORDER BY salary DESC;",
      "comparison_policy": {
        "order_sensitive": true
      }
    }

Это internal API, не frontend contract.

## /tasks/next

Cold-start пользователь получает стартовую задачу синхронно:

    HTTP/1.1 200 OK

После успешного решения задачи task-progress-service создаёт analysis state pending и ждёт analytics.skill_assessment.updated.

Пока анализ не завершён:

    HTTP/1.1 202 Accepted
    Retry-After: 5

    {
      "status": "pending",
      "error": "analysis_pending",
      "pending_task_id": "...",
      "pending_attempt_id": "...",
      "pending_since": "...",
      "wait_until": "...",
      "retry_after_seconds": 5,
      "analysis_state": {
        "status": "pending"
      }
    }

Когда analysis completed:

- wait_until очищается;
- learner model обновляется;
- next task сохраняется в user_next_task_recommendations;
- /tasks/next возвращает 200 OK.

## Kafka input

playground.execution.completed публикуется playground-service после server-side autograding.

Событие содержит:

- event_id;
- user_id;
- task_id;
- user_query;
- reference_query;
- execution_success;
- is_correct;
- user_result;
- reference_result;
- created_at.

reference_query и reference_result — internal event data. Они не отдаются frontend'у.

task-progress-service использует событие, чтобы:

1. сохранить попытку;
2. обновить basic learner model;
3. опубликовать task.checked;
4. если задача решена, создать pending analysis state;
5. опубликовать task.completed.

analytics.skill_assessment.updated публикуется analytics-service после LLM/intelligence analysis.

task-progress-service использует это событие, чтобы:

1. применить skill_scores к learner model;
2. сохранить model/prompt version в user_skill_assessment_versions;
3. обновить user-specific graph overlays/recommendations;
4. перевести analysis state в completed;
5. очистить wait_until;
6. пересчитать и сохранить persisted next task.

## Kafka output

    task.checked
    task.completed
    task.recommended
    learner_model.updated

task.checked и task.completed содержат internal context, включая reference_sql и actual/expected results. Это нужно analytics/LLM.

## Comparison policy

Поле tasks.order_sensitive задаёт, важен ли порядок строк при сравнении результата.

    order_sensitive=false
      строки сравниваются без учёта порядка

    order_sensitive=true
      порядок строк должен совпасть

reference_sql может содержать ORDER BY просто для детерминированного expected output. Это не означает автоматически, что задача order-sensitive. Источник истины — tasks.order_sensitive.

## Миграции

Миграции task-progress применяются compose job'ом task-progress-migrate по *.up.sql.

Текущая структура:

    000001_init.up.sql
    000002_graph_thresholds.up.sql
    000003_async_overrides_and_recommendations.up.sql
    000004_hint_state.up.sql
    000005_persisted_next_task.up.sql
    000006_task_comparison_policy.up.sql
    000007_seed_sql_tasks.up.sql

000007_seed_sql_tasks.up.sql содержит seed SQL-задач и связи task_skills.

task-progress-postgres больше не использует docker-entrypoint-initdb.d seed. Источник схемы и seed data — migrations в services/task-progress-service/migrations.

## Environment

    ENV=local
    HTTP_ADDR=:8082
    DATABASE_URL=postgres://task_progress:task_progress@task-progress-postgres:5432/task_progress?sslmode=disable
    JWT_SECRET=change-me-in-production
    ALLOW_GATEWAY_HEADERS=true

    KAFKA_ENABLED=true
    KAFKA_BROKERS=redpanda:9092
    KAFKA_GROUP_ID=task-progress-service

    KAFKA_TOPIC_PLAYGROUND_EXECUTION_COMPLETED=playground.execution.completed
    KAFKA_TOPIC_ANALYTICS_SKILL_ASSESSMENT_UPDATED=analytics.skill_assessment.updated
    KAFKA_TOPIC_TASK_CHECKED=task.checked
    KAFKA_TOPIC_TASK_COMPLETED=task.completed
    KAFKA_TOPIC_TASK_RECOMMENDED=task.recommended
    KAFKA_TOPIC_LEARNER_MODEL_UPDATED=learner_model.updated

    RECOMMENDATION_WAIT_TIMEOUT=2m

## Local commands

    go test ./...
    go run ./main.go

Проверить internal reference context:

    curl -sS http://localhost:8083/internal/tasks/00000000-0000-0000-0000-000000010003/reference | jq .
