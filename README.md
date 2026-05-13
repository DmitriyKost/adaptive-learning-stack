# Adaptive Learning local stack

Общий локальный стенд для микросервисов:

- `api-gateway`
- `auth-service`
- `playground-service`
- `task-progress-service`
- `analytics-service`
- PostgreSQL для `auth-service`
- PostgreSQL для `playground-service`
- PostgreSQL для `task-progress-service`
- ClickHouse для `analytics-service`
- Redpanda как Kafka-брокер

## Запуск

```bash
docker compose up --build
```

После первого запуска будут выполнены миграции:

- `auth-migrate` применяет миграции auth-сервиса;
- `playground-migrate` создает защищенную playground-БД, роли, `task_data` и демо-данные;
- `task-progress-migrate` создает графы, навыки, задачи и состояние прогресса;
- `clickhouse-migrate` создает аналитические таблицы в ClickHouse.

Gateway доступен на:

```text
http://localhost:8080
```

Прямые debug-порты сервисов:

```text
auth-service:          http://localhost:8081
playground-service:    http://localhost:8082
task-progress-service: http://localhost:8083
analytics-service:     http://localhost:8084
```

## Проверка готовности

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## Быстрый smoke-test

Регистрация:

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"password123"}'
```

Логин:

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.com","password":"password123"}' \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')

echo "$TOKEN"
```

Получить следующую задачу:

```bash
curl -s http://localhost:8080/tasks/next \
  -H "Authorization: Bearer $TOKEN"
```

Выполнить SQL через playground напрямую:

```bash
curl -s -X POST http://localhost:8080/playground/execute \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "task_id":"00000000-0000-0000-0000-000000010001",
    "user_query":"SELECT id, name FROM task_data.employees ORDER BY id;",
    "reference_query":"SELECT id, name FROM task_data.employees ORDER BY id;"
  }'
```

После события `playground.execution.completed` сервис `task-progress` проверит результат и отправит события в аналитику. После успешного решения `/tasks/next` может вернуть `409 analysis_pending` до получения LLM-рекомендаций от analytics или до истечения soft timeout.


## Запуск с внешним интеллектуальным модулем (`diploma`)

В этом архиве `adaptive-learning-stack/` и `diploma/` лежат рядом, поэтому Linux/Void-запуск не требует Windows-путей:

```bash
cd adaptive-learning-stack
cp .env.example .env   # необязательно, только если хотите менять пути/порты
```

Для быстрого smoke-теста можно использовать Ollama вместо Hugging Face base + LoRA. В `.env.example` уже стоит:

```dotenv
LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://host.docker.internal:11434
OLLAMA_MODEL=llama3.1:8b-instruct-q6_K
```

На Linux/Void Ollama на хосте должен слушать не только `127.0.0.1`, иначе контейнер `intelligence` его не увидит. Для локального теста:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
ollama pull llama3.1:8b-instruct-q6_K
```

Затем запускайте стек:

```bash
docker compose -f docker-compose.yml -f docker-compose.with-diploma.yml up --build
```

Если нужен исходный LoRA-режим, поставьте `LLM_PROVIDER=local`, положите базовую Hugging Face-модель в `../diploma/models/base` или переопределите путь через `DIPLOMA_MODELS_HOST` в `.env`. LoRA уже ожидается в `../diploma/lora_sql_mastery`.

После запуска:

```bash
curl http://localhost:8090/health     # Python intelligence напрямую
curl http://localhost:8090/ready      # проверка backend-провайдера: Ollama или local model
curl http://localhost:8084/health     # analytics-service
curl http://localhost:8080/health     # api-gateway
```

`analytics-service` всегда использует внешний intelligence HTTP API:

```text
INTELLIGENCE_BASE_URL=http://intelligence:8080
INTELLIGENCE_READY_PATH=/ready
INTELLIGENCE_HTTP_TIMEOUT=600s
HINT_GENERATION_TIMEOUT=600s
```

Если запускаете Python-модуль отдельно на хосте, оставьте основной `docker-compose.yml` и задайте для `analytics-service` `INTELLIGENCE_BASE_URL` на адрес, доступный из контейнера. Для Linux обычно удобнее поднимать `intelligence` в той же compose-сети через `docker-compose.with-diploma.yml`, а не использовать `host.docker.internal`.

## Перезапуск с чистыми данными

```bash
docker compose down -v
```

Затем снова:

```bash
docker compose up --build
```

## Важные настройки

Все env-файлы лежат в `env/`:

- `auth-service.env`
- `playground-service.env`
- `task-progress-service.env`
- `analytics-service.env`
- `api-gateway.env`

Общий `JWT_SECRET` должен совпадать в `auth-service`, `api-gateway`, `task-progress-service` и `playground-service`.

В compose используется внутренний Kafka broker address:

```text
redpanda:9092
```

Для подключения с хоста используй:

```text
localhost:19092
```

## Troubleshooting: playground_app role

If `playground-service` fails with `Role "playground_app" does not exist`, use the fixed compose file from this archive and restart from clean volumes:

```bash
docker compose down -v --remove-orphans
docker compose up --build
```

The `playground-migrate` container now bootstraps and verifies the runtime roles before applying migrations.

## Playground migration note

The secure playground schema is also mounted into `playground-postgres` as init scripts. Always run `docker compose down -v --remove-orphans` before switching stack archives so PostgreSQL re-runs `/docker-entrypoint-initdb.d` on a clean volume.

To verify playground DB:

```bash
docker compose exec playground-postgres \
  psql -U postgres -d playground \
  -c "SELECT to_regnamespace('playground_internal'), to_regprocedure('playground_internal.ensure_user_workspace(uuid)'), to_regclass('task_data.employees');"
```


### Kafka consumer groups

`task-progress-service` uses separate Kafka consumer groups for incoming topics:

- `task-progress-service-playground` for `playground.execution.completed`;
- `task-progress-service-analytics` for `analytics.skill_assessment.updated`.

This avoids group rebalancing conflicts when the service consumes independent topics with separate readers.
