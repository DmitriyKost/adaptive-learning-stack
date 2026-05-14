# playground-service

Сервис безопасного выполнения SQL-запросов пользователя для SQL-тренажера.

Сервис не хранит прогресс пользователя. Он только:

- создает/поддерживает личный PostgreSQL schema/namespace пользователя;
- выполняет пользовательский SQL в изолированной роли;
- дает пользователю права на `SELECT/INSERT/UPDATE/DELETE/CREATE TABLE/DROP TABLE/TRUNCATE TABLE` только в его личном schema;
- дает read-only доступ к общему schema `task_data` с данными заданий;
- выполняет эталонный `reference_sql` из task-progress только в read-only режиме;
- публикует событие выполнения в Kafka/Redpanda.

## API

- `GET /health`
- `POST /execute`
- `GET /playground/workspace`
- `POST /playground/reset`

Все endpoints, кроме `/health`, требуют авторизацию.

По умолчанию сервис проверяет JWT access token, выпущенный `auth-service`. Также можно включить доверие к заголовкам API Gateway:

```env
TRUSTED_GATEWAY_HEADERS=true
```

В этом режиме сервис принимает:

```text
X-User-ID
X-User-Role
```

## POST /execute

```json
{
  "task_id": "task-1",
  "user_query": "SELECT * FROM products"
}
```

`user_id` не передается в body. Он берется из JWT или из заголовка `X-User-ID`, чтобы клиент не мог выполнить запрос от имени другого пользователя.

Ответ:

```json
{
  "event_id": "uuid",
  "user_id": "uuid",
  "task_id": "task-1",
  "user_result": {
    "columns": ["id", "name", "category", "price"],
    "rows": [],
    "rows_affected": 0,
    "query_time_ms": 3,
    "truncated": false
  },
  "reference_result": {
    "columns": ["id", "name", "category", "price"],
    "rows": [],
    "rows_affected": 0,
    "query_time_ms": 2,
    "truncated": false
  },
  "created_at": "2026-01-01T00:00:00Z"
}
```

## Kafka/Redpanda event

Topic по умолчанию:

```env
KAFKA_EXECUTION_TOPIC=playground.execution.completed
```

Тип события:

```text
playground.execution.completed
```

Событие содержит:

- `event_id`
- `event_type`
- `event_version`
- `user_id`
- `task_id`
- `user_query`
- `reference_query` в legacy event payload заполняется серверным `reference_sql` из task-progress; клиентский body игнорируется.
- `user_result`
- `reference_result`
- `created_at`

`task-progress` может подписаться на этот topic и сравнить `user_result` с серверным `reference_result`, сохранить попытку, обновить модель обучающегося и инициировать async-выбор следующей задачи после LLM-оценки.

## Переменные окружения

```env
ENV=local
HTTP_ADDR=:8080
DATABASE_URL=postgres://playground_app:playground_app@localhost:5432/playground?sslmode=disable
JWT_SECRET=change-me-in-production
TRUSTED_GATEWAY_HEADERS=false
KAFKA_BROKERS=localhost:9092
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
```

## Миграции

Миграция `000001_secure_playground.up.sql` должна запускаться от роли с правами `CREATEROLE`, потому что она создает:

- `playground_app` — runtime login для сервиса;
- `playground_readonly` — read-only роль для общего schema `task_data`;
- динамические роли `playground_user_<user_uuid>` для пользователей;
- динамические schema `u_<user_uuid>` для пользовательских данных.

Пример:

```bash
createdb playground
migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5432/playground?sslmode=disable" up
```

После миграции сам сервис должен подключаться как `playground_app`:

```env
DATABASE_URL=postgres://playground_app:playground_app@localhost:5432/playground?sslmode=disable
```

## Модель безопасности БД

Общие данные заданий хранятся в schema:

```text
task_data
```

Пользовательские данные хранятся в schema вида:

```text
u_<user_uuid_without_hyphens>
```

Для каждого пользователя создается отдельная PostgreSQL-роль:

```text
playground_user_<user_uuid_without_hyphens>
```

При выполнении пользовательского SQL сервис делает внутри транзакции:

```sql
SET LOCAL ROLE "playground_user_<uuid>";
SET LOCAL search_path = "u_<uuid>", task_data, public;
```

В результате:

- unqualified `CREATE TABLE`, `INSERT`, `UPDATE`, `DELETE` работают в личном schema пользователя;
- `SELECT` из `task_data` разрешен;
- изменение `task_data` запрещено правами PostgreSQL;
- доступ к schema другого пользователя запрещен.

## Запуск

```bash
go mod tidy
go run ./main.go
```

## Пример с JWT

```bash
curl -X POST http://localhost:8080/execute \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "task_id":"task-1",
    "user_query":"SELECT * FROM products"
  }'
```

## Пример через API Gateway headers

```bash
TRUSTED_GATEWAY_HEADERS=true go run ./main.go

curl -X POST http://localhost:8080/execute \
  -H 'X-User-ID: 6a38a380-236f-4f36-a692-10e01b3954db' \
  -H 'X-User-Role: student' \
  -H 'Content-Type: application/json' \
  -d '{
    "task_id":"task-1",
    "user_query":"CREATE TABLE notes(id bigserial primary key, text text)"
  }'
```

## Docker Compose example

В архиве есть `docker-compose.example.yml` с PostgreSQL, Redpanda и сервисом. Перед запуском сервиса все равно нужно применить миграцию к PostgreSQL от роли `postgres`, чтобы создать runtime role `playground_app`.
