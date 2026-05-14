# Adaptive Learning Stack

Локальный dev-стенд для адаптивного SQL-курса.

Стек состоит из:

- `api-gateway` — единая публичная точка входа;
- `auth-service` — регистрация, login, refresh, JWT;
- `playground-service` — безопасное выполнение SQL пользователя и server-side автопроверка;
- `task-progress-service` — задачи, прогресс, learner model, граф навыков, выбор следующей задачи;
- `analytics-service` — event log, ClickHouse, подготовка контекста и интеграция с внешним intelligence service;
- `intelligence` из внешнего модуля `adaptive_sql_diploma` — LLM/LoRA/Ollama-интеллект;
- PostgreSQL для auth/playground/task-progress;
- ClickHouse для analytics;
- Redpanda как Kafka-compatible broker.

## Запуск

Обычный dev-запуск:

    docker compose up -d --build

Запуск с внешним модулем `adaptive_sql_diploma`:

    docker compose \
      -f docker-compose.yml \
      -f docker-compose.with-adaptive-sql-adaptive_sql_diploma.yml \
      up -d --build

Запуск с GPU override:

    docker compose \
      -f docker-compose.yml \
      -f docker-compose.with-adaptive-sql-adaptive_sql_diploma.yml \
      -f docker-compose.with-adaptive-sql-adaptive_sql_diploma.gpu.yml \
      up -d --build

Полный reset dev-данных:

    docker compose \
      -f docker-compose.yml \
      -f docker-compose.with-adaptive-sql-adaptive_sql_diploma.yml \
      -f docker-compose.with-adaptive-sql-adaptive_sql_diploma.gpu.yml \
      down -v --remove-orphans

## Основной public API flow

Frontend работает только через gateway:

    http://localhost:8080

Новый пользователь:

    POST /auth/register
    GET  /tasks/next

`GET /tasks/next` для нового пользователя сразу возвращает стартовую задачу. После успешного решения задачи следующий выбор становится асинхронным:

    POST /playground/execute
    GET  /tasks/next

Если LLM-анализ ещё идёт, `/tasks/next` возвращает:

    HTTP/1.1 202 Accepted
    Retry-After: 5

Тело ответа:

    {
      "status": "pending",
      "error": "analysis_pending",
      "retry_after_seconds": 5,
      "analysis_state": {
        "status": "pending"
      }
    }

Когда следующая задача готова:

    HTTP/1.1 200 OK

    {
      "task": {
        "id": "...",
        "title": "...",
        "description": "...",
        "difficulty": "easy",
        "skills": [
          { "skill_code": "select", "weight": 1 }
        ]
      },
      "reason": "...",
      "score": 1.23,
      "graph_code": "core_sql",
      "professional_track": "core",
      "repeat_mode": false,
      "recommended_skills": []
    }

## Выполнение SQL

Публичный request:

    POST /playground/execute

    {
      "task_id": "00000000-0000-0000-0000-000000010001",
      "user_query": "SELECT id, name FROM task_data.employees ORDER BY id;"
    }

Клиент не передаёт `reference_query`. Если поле `reference_query` или любое другое неизвестное поле передано, request отклоняется.

Публичный response:

    {
      "event_id": "...",
      "user_id": "...",
      "task_id": "...",
      "execution_success": true,
      "is_correct": true,
      "user_result": {
        "columns": ["id", "name"],
        "rows": [
          { "id": 1, "name": "Ivan" }
        ],
        "rows_affected": 1,
        "query_time_ms": 2,
        "truncated": false
      },
      "created_at": "2026-05-14T19:29:44Z"
    }

Публичный response не содержит `reference_sql` и не содержит `reference_result`.

## Reference SQL и автопроверка

`reference_sql` хранится в `task-progress-service` и доступен только внутреннему backend flow.

Путь проверки:

    frontend
    -> POST /playground/execute { task_id, user_query }
    -> playground-service запрашивает internal task reference у task-progress-service
    -> playground-service выполняет user SQL
    -> playground-service выполняет reference SQL read-only
    -> playground-service сравнивает результаты по comparison policy
    -> playground-service публикует playground.execution.completed
    -> task-progress-service обновляет learner model
    -> analytics-service запускает LLM analysis
    -> task-progress-service фиксирует следующую задачу

`task-progress-service` владеет policy проверки задачи. Сейчас поддерживается:

    {
      "comparison_policy": {
        "order_sensitive": true
      }
    }

Если `order_sensitive=false`, строки сравниваются без учёта порядка. Если `order_sensitive=true`, порядок строк является частью ответа.

## Отключённый legacy endpoint

Старый endpoint:

    POST /tasks/{id}/submit

отключён и возвращает:

    HTTP/1.1 410 Gone

    { "error": "legacy_submit_disabled" }

Единственный публичный submit path — `POST /playground/execute`.

## Проверки

Главный smoke-test:

    MAX_WAIT_SECONDS=600 ./scripts/smoke-e2e-llm.sh

Contract/security check:

    MAX_WAIT_SECONDS=600 ./scripts/checks/api-contract-check.sh

Проверка order-sensitive задач:

    ./scripts/checks/order-sensitive-check.sh

Benchmark ожидания следующей задачи:

    RUNS=5 \
    POLL_INTERVAL_SECONDS=0.2 \
    MAX_WAIT_SECONDS=600 \
    ./scripts/checks/benchmark-next-task-wait.sh

## Intelligence service

При запуске с `docker-compose.with-adaptive-sql-adaptive_sql_diploma.yml` `analytics-service` вызывает внешний Python intelligence service:

    GET  /ready
    POST /v1/assessments/evaluate
    POST /v1/hints/generate

Основные переменные:

    INTELLIGENCE_BASE_URL=http://intelligence:8080
    INTELLIGENCE_EVALUATE_PATH=/v1/assessments/evaluate
    INTELLIGENCE_HINT_PATH=/v1/hints/generate
    INTELLIGENCE_READY_PATH=/ready
    INTELLIGENCE_HTTP_TIMEOUT=600s
    INTELLIGENCE_INCLUDE_CAREER=false

Проверка:

    curl -sS http://localhost:8090/ready | jq .
    curl -sS http://localhost:8084/ready | jq .
    curl -sS http://localhost:8080/ready | jq .

## Миграции task-progress

Миграции применяются compose job'ом `task-progress-migrate` по `*.up.sql`.

Текущая структура:

    000001_init.up.sql
    000002_graph_thresholds.up.sql
    000003_async_overrides_and_recommendations.up.sql
    000004_hint_state.up.sql
    000005_persisted_next_task.up.sql
    000006_task_comparison_policy.up.sql
    000007_seed_sql_tasks.up.sql

Задачи seed'ятся отдельной миграцией `000007_seed_sql_tasks.up.sql`.

## Dev notes

В dev-compose некоторые сервисы и БД могут быть проброшены на host для тестирования. Production/deploy-конфигурация должна публиковать наружу только gateway и необходимые внешние entrypoints.
