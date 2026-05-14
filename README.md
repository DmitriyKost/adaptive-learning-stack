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
    "user_query":"SELECT id, name FROM task_data.employees ORDER BY id;"
  }'
```

`reference_sql` для автопроверки не принимается от клиента: `playground-service` всегда запрашивает эталонный SQL из `task-progress-service` и выполняет его в read-only режиме. После события `playground.execution.completed` сервис `task-progress` проверит результат и отправит события в аналитику. После успешного решения `/tasks/next` возвращает заранее сохраненную рекомендацию; пока LLM-оценка и async-выбор следующей задачи не завершены, endpoint возвращает `409 analysis_pending`.


## Запуск с внешним интеллектуальным модулем (`diploma`) и LoRA

Рекомендуемая структура каталогов:

```text
workspace/
  adaptive-learning-stack/
  diploma/
```

Внешний модуль запускается как Docker-контейнер `intelligence` в той же compose-сети. Он грузит Hugging Face base model из `../diploma/models/base` и LoRA из `../diploma/lora_sql_mastery`. Ollama для этого режима не используется.

### 1. Подготовить базовую модель

LoRA в `diploma/lora_sql_mastery/adapter_config.json` указывает на base model:

```text
unsloth/meta-llama-3.1-8b-instruct-unsloth-bnb-4bit
```

Скачайте её в Hugging Face формате, не GGUF/Ollama:

```bash
cd ../diploma
python3 -m pip install -U "huggingface_hub[cli]"
huggingface-cli download \
  unsloth/meta-llama-3.1-8b-instruct-unsloth-bnb-4bit \
  --local-dir models/base
```

Или из `adaptive-learning-stack`:

```bash
./scripts/download-diploma-base-model.sh
```

Если Hugging Face вернёт 401/403, сначала выполните `huggingface-cli login` и примите условия доступа к base model.

Если модель лежит в другом месте, задайте путь в `adaptive-learning-stack/.env`:

```dotenv
DIPLOMA_MODELS_HOST=/absolute/path/to/base-model
DIPLOMA_LORA_HOST=/absolute/path/to/lora_sql_mastery
```

### 2. Настроить compose env

```bash
cd ../adaptive-learning-stack
cp .env.example .env
```

Ключевые значения по умолчанию:

```dotenv
DIPLOMA_ROOT=../diploma
DIPLOMA_MODELS_HOST=../diploma/models/base
DIPLOMA_LORA_HOST=../diploma/lora_sql_mastery
LLM_LOAD_IN_4BIT=true
```

### 3. Запуск на NVIDIA GPU

Для Llama 3.1 8B 4-bit нужен Docker с NVIDIA Container Toolkit. Проверка на хосте:

```bash
nvidia-smi
docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi
```

Запуск полного стека:

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.with-diploma.yml \
  -f docker-compose.with-diploma.gpu.yml \
  up -d --build --force-recreate
```

CPU-режим оставлен только для маленьких совместимых моделей. Для него нужно выставить `LLM_LOAD_IN_4BIT=false`, но 8B-модель будет медленной и потребует много RAM.

### 4. Проверка readiness и прогрев модели

```bash
curl -sS http://localhost:8090/health | jq .
curl -sS http://localhost:8090/ready | jq .
```

`/ready` проверяет, что base model и LoRA примонтированы. Чтобы загрузить модель в память до E2E-теста:

```bash
curl -sS -X POST http://localhost:8090/warmup \
  -H 'Content-Type: application/json' \
  -d '{}' | jq .
```

### 5. Полный smoke-test

```bash
chmod +x scripts/smoke-e2e-llm.sh
MAX_WAIT_SECONDS=600 ./scripts/smoke-e2e-llm.sh
```

Smoke-test делает полный прогон нового пользователя:

```text
api-gateway → playground → Kafka → task-progress → Kafka → analytics-service → intelligence/LoRA → analytics.skill_assessment.updated → learner model → planner /tasks/next
```

Успех подтверждается строками:

```text
OK intelligence model loaded
OK LLM run completed: ...
OK planner returned next_task_id=...
OK E2E smoke test passed
```

`analytics-service` использует внешний intelligence HTTP API:

```text
INTELLIGENCE_BASE_URL=http://intelligence:8080
INTELLIGENCE_READY_PATH=/ready
INTELLIGENCE_HTTP_TIMEOUT=600s
HINT_GENERATION_TIMEOUT=600s
```

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
