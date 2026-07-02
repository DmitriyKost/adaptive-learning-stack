# playground-service

Сервис выполнения SQL пользователя и server-side автопроверки.

Он:

- создаёт и поддерживает личную PostgreSQL schema пользователя;
- выполняет user SQL в изолированной роли;
- даёт read-only доступ к schema task_data;
- запрашивает reference SQL и comparison policy у task-progress-service;
- выполняет reference SQL read-only;
- сравнивает user_result и reference_result внутри backend;
- возвращает пользователю только user_result и verdict;
- публикует internal event playground.execution.completed.

Сервис не хранит learner model и не выбирает следующую задачу.

## Public API

    GET  /health
    GET  /ready
    POST /playground/execute
    GET  /playground/workspace
    POST /playground/reset

Также может быть доступен alias:

    POST /execute

## Auth

Все endpoints, кроме /health и /ready, требуют авторизацию.

В gateway-mode сервис может доверять заголовкам:

    X-User-ID
    X-User-Role

Эти заголовки должны выставляться gateway, а не клиентом напрямую.

## POST /playground/execute

Request:

    {
      "task_id": "00000000-0000-0000-0000-000000010001",
      "user_query": "SELECT id, name FROM task_data.employees ORDER BY id;"
    }

Public request не содержит reference_query. Request body парсится strict decoder'ом:

- неизвестные поля отклоняются;
- trailing JSON после первого объекта отклоняется.

Перед выполнением user SQL сервис сначала валидирует task_id и получает reference context у task-progress-service.

Response:

    {
      "event_id": "...",
      "user_id": "...",
      "task_id": "...",
      "execution_success": true,
      "is_correct": true,
      "user_result": {
        "columns": ["id", "name"],
        "rows": [
          {
            "id": 1,
            "name": "Ivan"
          }
        ],
        "rows_affected": 1,
        "query_time_ms": 2,
        "truncated": false
      },
      "created_at": "..."
    }

Public response не содержит reference_sql и не содержит reference_result.

## Grading flow

    POST /playground/execute
    -> get task reference from task-progress internal endpoint
    -> ensure user workspace
    -> execute user SQL
    -> execute reference SQL read-only
    -> compare results using comparison policy
    -> return public verdict
    -> publish Kafka event with internal grading context

## Comparison policy

task-progress-service владеет policy проверки задачи.

Internal reference response содержит:

    {
      "task_id": "...",
      "reference_sql": "SELECT ...",
      "comparison_policy": {
        "order_sensitive": true
      }
    }

Если order_sensitive=false, строки сравниваются без учёта порядка.

Если order_sensitive=true, порядок строк должен совпасть.

Truncated результаты не засчитываются как correct.

## Kafka event

Topic:

    playground.execution.completed

Internal event содержит reference_query и reference_result, потому что они нужны task-progress-service и analytics-service.

Пример event payload:

    {
      "event_id": "...",
      "event_type": "playground.execution.completed",
      "event_version": 1,
      "user_id": "...",
      "task_id": "...",
      "user_query": "SELECT ...",
      "reference_query": "SELECT ...",
      "execution_success": true,
      "is_correct": true,
      "user_result": {
        "columns": ["id"],
        "rows": [
          {
            "id": 1
          }
        ],
        "truncated": false
      },
      "reference_result": {
        "columns": ["id"],
        "rows": [
          {
            "id": 1
          }
        ],
        "truncated": false
      },
      "created_at": "..."
    }

Это internal event, не public HTTP response.

## Environment

    ENV=local
    HTTP_ADDR=:8080
    DATABASE_URL=postgres://playground_app:playground_app@playground-postgres:5432/playground?sslmode=disable
    JWT_SECRET=change-me-in-production
    TRUSTED_GATEWAY_HEADERS=false

    TASK_PROGRESS_BASE_URL=http://task-progress-service:8082
    TASK_REFERENCE_TIMEOUT=3s
    RETURN_REFERENCE_RESULT=false

    KAFKA_BROKERS=redpanda:9092
    KAFKA_CLIENT_ID=playground-service
    KAFKA_EXECUTION_TOPIC=playground.execution.completed
    KAFKA_AUTO_CREATE_TOPICS=true

    EXECUTION_TIMEOUT=5s
    STATEMENT_TIMEOUT=3s
    LOCK_TIMEOUT=1s
    MAX_RESULT_ROWS=500

    TASK_SCHEMA=task_data
    INTERNAL_SCHEMA=playground_internal
    READONLY_ROLE=playground_readonly

RETURN_REFERENCE_RESULT=true допускается только для local debug. Для frontend-safe режима должно быть false.

## DB security model

Общие данные заданий находятся в schema:

    task_data

Пользовательская schema:

    u_<user_uuid_without_hyphens>

Пользовательская роль:

    playground_user_<user_uuid_without_hyphens>

User SQL выполняется с локальным search_path:

    SET LOCAL ROLE "playground_user_<uuid>";
    SET LOCAL search_path = "u_<uuid>", task_data, public;

Пользователь может работать со своей schema. task_data доступен read-only. Schema других пользователей недоступны.

## Local commands

    go test ./...
    go run ./main.go

Проверка через gateway:

    curl -sS -X POST http://localhost:8080/playground/execute \
      -H "Authorization: Bearer $ACCESS_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"task_id":"00000000-0000-0000-0000-000000010001","user_query":"SELECT id, name FROM task_data.employees ORDER BY id;"}' | jq .
