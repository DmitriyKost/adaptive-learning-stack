# analytics-service

Сервис аналитики для адаптивного SQL-курса. Он соответствует текущей архитектуре `task-progress-service v5`: получает события из Kafka/Redpanda, пишет raw и структурированные логи в ClickHouse, агрегирует контекст выполнения задачи и публикует ответное событие `analytics.skill_assessment.updated` для разблокировки выбора следующей задачи в `task-progress`.

## Внешний модуль интеллекта (Python, adaptive-sql-diploma)

`analytics-service` всегда вызывает внешний intelligence HTTP API и не имеет локального fallback-клиента.

1. Поднимите сервис из репозитория **adaptive-sql-diploma** (`uvicorn app.main:app`, см. его README), при необходимости задайте `ANALYTICS_API_KEY`.
2. В `analytics-service` задайте `INTELLIGENCE_BASE_URL` (например `http://127.0.0.1:8080`, `http://host.docker.internal:8090` из контейнера или `http://intelligence:8080` в compose-сети), тот же секрет в `INTELLIGENCE_API_KEY`, что и у Python.
3. Пути по умолчанию: `INTELLIGENCE_READY_PATH=/ready`, `INTELLIGENCE_EVALUATE_PATH=/v1/assessments/evaluate`, `INTELLIGENCE_HINT_PATH=/v1/hints/generate`. Таймаут: `INTELLIGENCE_HTTP_TIMEOUT` (по умолчанию 600s). Карьера в запросе оценки: `INTELLIGENCE_INCLUDE_CAREER=true`.

Реализация клиента: `internal/service/http_intelligence.go`.

## Роль сервиса

`task-progress` хранит только оперативное состояние модели обучающегося: текущие mastery/confidence, статусы задач, активный reference graph, user-specific overrides и последнее состояние ожидания аналитики. `analytics-service` хранит log-like часть модели:

- все входящие события в `raw_events`;
- все попытки решения задач с SQL, ошибками, временем выполнения и подсказками;
- события успешного выполнения с `graph_state`;
- логи выбора задачи `task.recommended`;
- логи изменения модели `learner_model.updated`;
- prepared context для внешнего интеллектуального модуля;
- request/response внешнего интеллектуального модуля;
- оценки mastery по тегам;
- рекомендации тегов;
- карьерные рекомендации.

## Входящие Kafka topics

```text
TASK_CHECKED=task.checked
TASK_COMPLETED=task.completed
TASK_RECOMMENDED=task.recommended
LEARNER_MODEL_UPDATED=learner_model.updated
```

Сервис коммитит Kafka offset только после успешной записи в ClickHouse и, для `task.completed`, после публикации `analytics.skill_assessment.updated`. Если обработка падает, сообщение не коммитится и Redpanda/Kafka сможет доставить его повторно.

## Исходящие Kafka topics

```text
analytics.skill_assessment.updated
analytics.career_recommendation.created
```

Главное исходящее событие для `task-progress`:

```json
{
  "event_type": "analytics.skill_assessment.updated",
  "user_id": "...",
  "source": "analytics-service/intelligence",
  "graph_code": "DA",
  "professional_track": "DA",
  "source_task_id": "...",
  "source_attempt_id": "...",
  "observed_until": "2026-05-11T12:00:00Z",
  "assessment_mode": "merge",
  "analysis": {
    "analysis_run_id": "...",
    "model_version": "sql-intelligence-v1",
    "prompt_version": "sql-mastery-context-v1",
    "seed": 42,
    "temperature": 0
  },
  "user_graph_overlay": {
    "graph_code": "DA",
    "professional_track": "DA",
    "skills": [
      {
        "skill_code": "group_by",
        "priority_weight": 1.4,
        "mastery_threshold": 0.78,
        "reason": "user-specific overlay from latest LLM tag recommendation"
      }
    ]
  },
  "recommended_skills": [
    {
      "skill_code": "group_by",
      "priority": 0.91,
      "recommended_action": "remediate",
      "reason": "gap=0.30, forgetting=0.12, threshold=0.80"
    }
  ],
  "skill_scores": [
    {
      "skill_code": "group_by",
      "mastery_score": 0.72,
      "confidence": 0.73,
      "components": {
        "correctness": 0.5,
        "independence": 0.75,
        "efficiency": 0.82
      }
    }
  ]
}
```

`recommended_skills` отправляется как полный snapshot. `task-progress v5` заменяет предыдущие рекомендации пользователя для активного графа этим набором.

## Обработка `task.completed`

Полный pipeline:

1. `task.completed` сохраняется в ClickHouse.
2. Сервис загружает все `task.checked` по `user_id + task_id`.
3. Формирует контекст для внешнего intelligence API:
   - task description;
   - reference SQL;
   - expected/actual result;
   - все попытки пользователя;
   - SQL каждой попытки;
   - ошибки СУБД;
   - hint requested/used/type/count;
   - `graph_state` с навыками, порогами, effective mastery, dependencies;
   - последние события `task.recommended`;
   - последние `learner_model.updated`.
4. Внешний модуль возвращает `mastery_score`, `recommended_skills`, `user_graph_overlay` и, при включённом `INTELLIGENCE_INCLUDE_CAREER`, карьерную рекомендацию.
5. Сервис сохраняет request/response в `llm_analysis_runs` и публикует `analytics.skill_assessment.updated`.

## Reference graphs read-only

Сервис аналитики не изменяет reference graphs. Он не отправляет `graph_patch` для мутации `learning_graphs`, `graph_skills`, `skill_dependencies`. Любая рекомендация графа отправляется только как `user_graph_overlay`, который `task-progress` применяет в user-specific таблицы:

```text
user_graph_skill_overrides
user_skill_dependency_overrides
```

## ClickHouse

Миграция находится в:

```text
migrations/000001_clickhouse_init.sql
```

Можно выполнить вручную:

```bash
clickhouse-client --multiquery --database analytics < migrations/000001_clickhouse_init.sql
```

Или включить автоматическое создание таблиц:

```env
CLICKHOUSE_AUTO_MIGRATE=true
```

## API

```text
GET /health
GET /ready
```

## Запуск

```bash
go mod tidy
go test ./...
docker compose -f docker-compose.example.yml up --build
```

## Важное для интеграции с task-progress

`task-progress` блокирует `GET /tasks/next` после успешного решения задачи до ответа аналитики или до истечения soft timeout. Этот сервис публикует `analytics.skill_assessment.updated` синхронно в рамках обработки `task.completed`; после получения этого события `task-progress` завершает `user_analysis_state = pending` и разрешает выбор следующей задачи.

## Hint generation

`analytics-service` owns hint generation because it has the ClickHouse event log and can aggregate the context required by the external intelligence module.

Internal endpoint:

```http
POST /internal/hints/generate
```

Request:

```json
{
  "user_id": "...",
  "task_id": "...",
  "current_attempt_number": 2,
  "request_id": "..."
}
```

The service loads all `task.checked` attempts for the user/task from ClickHouse, adds task metadata, reference SQL, previous errors, previous hints and calls the external intelligence client. Successful hints are saved to `hint_events` and published as `analytics.hint.generated`.

If the external module times out or fails, the response is:

```json
{"error":"hint_generation_failed"}
```

Failed hints are intentionally not written to `hint_events`, because no hint was actually shown to the user.
