# api-gateway

Единая публичная точка входа для Adaptive Learning Stack.

Gateway:

- валидирует JWT access token от auth-service;
- удаляет клиентские X-User-ID и X-User-Role;
- выставляет downstream-заголовки пользователя после проверки JWT;
- проксирует public API в auth-service, task-progress-service, playground-service и analytics-service;
- не является владельцем бизнес-логики задач, прогресса или SQL-проверки.

## Public API

Служебные endpoints:

    GET /health
    GET /ready

Auth:

    POST /auth/register
    POST /auth/login
    POST /auth/refresh
    POST /auth/logout

Users:

    GET /users/me
    GET /users/{id}

Tasks and progress:

    GET  /tasks
    GET  /tasks/{id}
    GET  /tasks/next
    POST /tasks/{id}/hint

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

Playground:

    POST /playground/execute
    GET  /playground/workspace
    POST /playground/reset

## Frontend flow

Новый пользователь:

    POST /auth/register
    GET  /tasks/next

Cold-start /tasks/next должен вернуть стартовую задачу сразу:

    HTTP/1.1 200 OK

После успешного submit следующая задача выбирается асинхронно:

    POST /playground/execute
    GET  /tasks/next

Пока LLM-анализ ещё идёт:

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
          {
            "skill_code": "select",
            "weight": 1
          }
        ]
      },
      "reason": "...",
      "score": 1.23,
      "graph_code": "core_sql",
      "professional_track": "core",
      "repeat_mode": false,
      "recommended_skills": []
    }

## Playground execute contract

Единственный public submit path:

    POST /playground/execute

Request:

    {
      "task_id": "00000000-0000-0000-0000-000000010001",
      "user_query": "SELECT id, name FROM task_data.employees ORDER BY id;"
    }

Клиент не передаёт reference_query. Неизвестные поля отклоняются.

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

## Disabled legacy endpoint

Старый endpoint отключён:

    POST /tasks/{id}/submit

Ожидаемый ответ:

    HTTP/1.1 410 Gone

    {
      "error": "legacy_submit_disabled"
    }

Frontend не должен использовать этот endpoint.

## Environment

    ENV=local
    HTTP_ADDR=:8080
    JWT_SECRET=change-me-in-production
    REQUEST_TIMEOUT=30s
    SHUTDOWN_TIMEOUT=10s

    AUTH_SERVICE_URL=http://auth-service:8080
    TASK_PROGRESS_SERVICE_URL=http://task-progress-service:8082
    PLAYGROUND_SERVICE_URL=http://playground-service:8080
    ANALYTICS_SERVICE_URL=http://analytics-service:8083

    FORWARD_AUTHORIZATION=true
    CHECK_DOWNSTREAM_READY=true

    CORS_ALLOWED_ORIGINS=*
    CORS_ALLOWED_HEADERS=Authorization,Content-Type,X-Request-ID
    CORS_ALLOWED_METHODS=GET,POST,PUT,PATCH,DELETE,OPTIONS

JWT_SECRET должен совпадать между сервисами, которые валидируют JWT самостоятельно.

## Security boundary

Клиентские X-User-ID и X-User-Role не являются доверенными. Gateway удаляет их и выставляет заново после JWT validation.

Internal endpoints вида /internal/... не являются public API gateway.
