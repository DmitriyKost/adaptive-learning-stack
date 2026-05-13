# api-gateway

Единая точка входа для микросервисов адаптивного SQL-курса.

Gateway не хранит бизнес-данные и не ходит в БД. Он:

- валидирует JWT access token, выпущенный `auth-service`;
- удаляет клиентские `X-User-ID` / `X-User-Role`, чтобы клиент не мог подменить пользователя;
- добавляет доверенные заголовки для downstream-сервисов:
  - `X-User-ID`;
  - `X-User-Role`;
  - `X-Request-ID`;
- проксирует запросы в нужный сервис;
- не проксирует внутренний API аналитики `/internal/hints/generate` наружу;
- возвращает `Retry-After` от `task-progress`, когда `/tasks/next` отвечает `409 analysis_pending`.

## Downstream-сервисы

```text
auth-service          -> регистрация, login, refresh, users
task-progress-service -> задачи, прогресс, графы, подсказка как пользовательский endpoint
playground-service    -> выполнение SQL
analytics-service     -> внутренние логи, агрегации, LLM, генерация подсказок
```

## API

### Public

```text
GET  /health
GET  /ready

POST /auth/register
POST /auth/login
POST /auth/refresh
POST /auth/logout
```

### Auth service через gateway

```text
GET /users/me
GET /users/{id}
```

### Task-progress service через gateway

```text
GET  /tasks
GET  /tasks/{id}
GET  /tasks/next
POST /tasks/{id}/submit
POST /tasks/{id}/hint

GET  /progress/me
GET  /progress/skills

GET  /users/me/learning-graph
PUT  /users/me/learning-graph

GET  /skills
POST /skills                       admin only

GET  /graphs
POST /graphs                       admin only
POST /graphs/{id}/skills           admin only
POST /graphs/{id}/dependencies     admin only
```

### Playground service через gateway

```text
POST /execute
POST /playground/execute
GET  /playground/workspace
POST /playground/reset
```

`workspace/reset` оставлены в маршрутах gateway как контрактный API. Если текущая версия `playground-service` их еще не реализует, gateway просто проксирует запрос, а сам сервис вернет `404` или `405`.

### Analytics service

Пользовательские endpoints аналитики сейчас не открываются наружу. Внутренний endpoint:

```text
POST /internal/hints/generate
```

остается доступен только для `task-progress-service` внутри сети сервисов.

## Переменные окружения

```env
ENV=local
HTTP_ADDR=:8080
JWT_SECRET=change-me-in-production
REQUEST_TIMEOUT=30s
SHUTDOWN_TIMEOUT=10s

AUTH_SERVICE_URL=http://auth-service:8080
TASK_PROGRESS_SERVICE_URL=http://task-progress-service:8082
PLAYGROUND_SERVICE_URL=http://playground-service:8081
ANALYTICS_SERVICE_URL=http://analytics-service:8083

FORWARD_AUTHORIZATION=true
CHECK_DOWNSTREAM_READY=true

CORS_ALLOWED_ORIGINS=*
CORS_ALLOWED_HEADERS=Authorization,Content-Type,X-Request-ID
CORS_ALLOWED_METHODS=GET,POST,PUT,PATCH,DELETE,OPTIONS
```

Важно: `JWT_SECRET` должен совпадать с `auth-service`, `task-progress-service` и `playground-service`, если downstream-сервисы сами проверяют JWT.

Если downstream-сервисам разрешено доверять gateway headers, включи у них:

```env
ALLOW_GATEWAY_HEADERS=true             # task-progress-service
TRUSTED_GATEWAY_HEADERS=true           # playground-service, если поддерживается этой версией
```

При этом gateway все равно может оставлять `Authorization` через `FORWARD_AUTHORIZATION=true`.

## Запуск локально

```bash
go test ./...
go run ./main.go
```

## Примеры

Регистрация:

```bash
curl -X POST http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"strongpass123"}'
```

Login:

```bash
curl -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"strongpass123"}'
```

Получить следующую задачу:

```bash
curl http://localhost:8080/tasks/next \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Если `task-progress` ожидает ответ analytics/LLM после успешного решения, gateway вернет ответ downstream-сервиса:

```json
{
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
```

Запросить подсказку после первого submit:

```bash
curl -X POST http://localhost:8080/tasks/$TASK_ID/hint \
  -H "Authorization: Bearer $ACCESS_TOKEN"
```

Выполнить SQL в playground:

```bash
curl -X POST http://localhost:8080/execute \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "task_id":"00000000-0000-0000-0000-000000010001",
    "dataset_id":"default",
    "sql":"SELECT * FROM task_data.products"
  }'
```

## Безопасность

Клиент не должен передавать `X-User-ID` и `X-User-Role`. Gateway всегда удаляет эти заголовки и выставляет их заново на основе валидного JWT.

Reference graphs не изменяются analytics/LLM через gateway. Админские изменения графов проходят только через admin-protected endpoints `POST /graphs`, `POST /graphs/{id}/skills`, `POST /graphs/{id}/dependencies`.
