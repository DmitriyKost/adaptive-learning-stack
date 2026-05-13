# task-progress-service

Сервис прогресса и задач для адаптивного SQL-курса.

`auth-service` хранит только учетные записи, роли и токены. `task-progress-service` хранит оперативную часть модели обучающегося: статусы задач, счетчики попыток, оценки навыков, confidence, активный reference graph, пользовательские overrides графа, рекомендации по навыкам и состояние ожидания аналитики. Полные execution logs, SQL-решения, ошибки, логи выбора задачи и история подсказок должны храниться в `analytics-service`.

## Что реализовано

- каталог `skills`, `tasks`, `task_skills`;
- reference graphs: `learning_graphs`, `graph_skills`, `skill_dependencies`;
- изменение reference graphs только через admin API;
- user-specific graph overrides от analytics/LLM:
  - `user_graph_skill_overrides`;
  - `user_skill_dependency_overrides`;
- рекомендации тегов/навыков от analytics/LLM:
  - `user_skill_recommendations`;
  - рекомендации хранятся как последний snapshot по `user_id + graph_id`: новый ответ analytics полностью заменяет предыдущий набор;
- компактная модель обучающегося:
  - `user_skills`;
  - `user_task_status`;
  - `user_learning_profiles`;
- защита асинхронного сценария:
  - после успешного решения создается `user_analysis_state = pending`;
  - `GET /tasks/next` возвращает `409 analysis_pending`, пока не придет `analytics.skill_assessment.updated` или пока ожидание не истечет;
  - timeout policy — `soft`: после истечения `RECOMMENDATION_WAIT_TIMEOUT` выбор следующей задачи разблокируется, но поздний analytics/LLM-ответ все равно применяется, если он не устарел по `observed_until`;
  - `409 analysis_pending` содержит `pending_task_id`, `pending_attempt_id`, `wait_until` и `retry_after_seconds`;
- защита от устаревших LLM-оценок:
  - `user_skill_assessment_versions`;
  - assessment не применяется, если его `observed_until` старее уже примененного по навыку;
- базовая кривая Эббингауза по `user_skills.updated_at` только для выбора задач и `graph_state`;
- Kafka/Redpanda consumer:
  - `playground.execution.completed`;
  - `analytics.skill_assessment.updated`;
- Kafka/Redpanda publisher:
  - `task.checked`;
  - `task.completed`;
  - `task.recommended`;
  - `learner_model.updated`.

## API

```text
GET  /health
GET  /ready

GET  /tasks
GET  /tasks/{id}
GET  /tasks/next
POST /tasks/{id}/submit

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

## Переменные окружения

```env
ENV=local
HTTP_ADDR=:8082
DATABASE_URL=postgres://task_progress:task_progress@localhost:5434/task_progress?sslmode=disable
JWT_SECRET=change-me-in-production
ALLOW_GATEWAY_HEADERS=false

DEFAULT_GRAPH_CODE=core_sql
MASTERY_THRESHOLD=0.70
PREREQUISITE_THRESHOLD=0.60
RECALL_THRESHOLD=0.35
RECOMMENDATION_WAIT_TIMEOUT=2m
MIN_HALF_LIFE_DAYS=1
MAX_HALF_LIFE_DAYS=21

KAFKA_ENABLED=true
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=task-progress-service
KAFKA_GROUP_ID=task-progress-service
KAFKA_TOPIC_PLAYGROUND_EXECUTION_COMPLETED=playground.execution.completed
KAFKA_TOPIC_ANALYTICS_SKILL_ASSESSMENT_UPDATED=analytics.skill_assessment.updated
KAFKA_TOPIC_TASK_CHECKED=task.checked
KAFKA_TOPIC_TASK_COMPLETED=task.completed
KAFKA_TOPIC_TASK_RECOMMENDED=task.recommended
KAFKA_TOPIC_LEARNER_MODEL_UPDATED=learner_model.updated
```

## Миграции

```bash
migrate -path ./migrations -database "$DATABASE_URL" up
```

Миграции создают:

- `core_sql`, `DA`, `DI` как reference graphs;
- graph-specific пороги `graph_skills.mastery_threshold`;
- dependency-specific пороги `skill_dependencies.required_mastery`;
- per-user overrides и рекомендации навыков;
- `user_analysis_state` для блокировки выбора следующей задачи до ответа analytics.

## Reference graphs и user overrides

Reference graph — это базовая траектория. Ее меняет только админ:

```text
POST /graphs
POST /graphs/{id}/skills
POST /graphs/{id}/dependencies
```

Analytics/LLM не изменяет `learning_graphs`, `graph_skills`, `skill_dependencies` напрямую. Входящее поле `graph_patch` оставлено для совместимости, но теперь трактуется как пользовательский overlay и записывается только в:

```text
user_graph_skill_overrides
user_skill_dependency_overrides
```

Это защищает систему от ситуации, когда рекомендация для одного пользователя меняет глобальный граф `DA` или `DI` для всех.

## Входящее событие от playground-service

Topic: `playground.execution.completed`.

Пример:

```json
{
  "event_id": "22222222-2222-2222-2222-222222222222",
  "event_version": 1,
  "event_type": "playground.execution.completed",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "task_id": "00000000-0000-0000-0000-000000010001",
  "attempt_id": "33333333-3333-3333-3333-333333333333",
  "success": true,
  "execution_time_ms": 12,
  "row_count": 4,
  "submitted_sql": "SELECT id, name FROM task_data.employees ORDER BY id",
  "columns": ["id", "name"],
  "rows": [[1, "Ivan"], [2, "Anna"]],
  "expected_columns": ["id", "name"],
  "expected_rows": [[1, "Ivan"], [2, "Anna"]],
  "hint_requested": true,
  "hint_used": true,
  "hint_type": "logical",
  "hints_count": 1,
  "created_at": "2026-05-11T12:00:00Z"
}
```

После обработки `task-progress`:

1. проверяет корректность результата;
2. обновляет `user_task_status` и `user_skills`;
3. публикует `task.checked` с полным task context:
   - `reference_sql`;
   - `task.description`;
   - `task.skills`;
   - `execution.submitted_sql`;
   - `actual_result`;
   - `expected_result`;
   - `hint`;
4. если задача решена, создает `user_analysis_state = pending`;
5. публикует `task.completed` с `graph_state`.

## task.checked

Событие обогащено данными, которые нужны analytics/LLM для сборки полного контекста:

```json
{
  "event_type": "task.checked",
  "user_id": "...",
  "task_id": "...",
  "reference_sql": "SELECT ...",
  "task": {
    "title": "...",
    "description": "...",
    "difficulty": "medium",
    "dataset_id": "...",
    "reference_sql": "SELECT ...",
    "skills": [
      { "skill_code": "group_by", "weight": 0.8 }
    ]
  },
  "execution": {
    "attempt_id": "...",
    "attempt_number": 2,
    "execution_success": true,
    "is_correct": false,
    "submitted_sql": "SELECT ...",
    "error_type": "syntax_error",
    "error_message": "...",
    "actual_result": { "columns": [], "rows": [] },
    "expected_result": { "columns": [], "rows": [] }
  },
  "hint": {
    "hint_requested": true,
    "hint_used": true,
    "hint_type": "logical",
    "hints_count": 1
  }
}
```

## task.completed

Публикуется только при успешном решении. Кроме task/execution/hint контекста содержит `graph_state`:

```json
{
  "event_type": "task.completed",
  "source_event_id": "22222222-2222-2222-2222-222222222222",
  "user_id": "...",
  "task_id": "...",
  "attempt_id": "...",
  "analysis_state": {
    "status": "pending",
    "pending_task_id": "...",
    "pending_attempt_id": "...",
    "wait_until": "2026-05-11T12:02:00Z"
  },
  "graph_state": {
    "profile": { "graph_code": "DA", "professional_track": "DA" },
    "average_mastery_score": 0.71,
    "average_effective_score": 0.64,
    "average_threshold_gap": 0.13,
    "skills": [],
    "dependencies": [],
    "recommended_skills": []
  }
}
```

`analytics-service` должен использовать `task.completed.source_event_id`, `task_id` и `attempt_id`, чтобы отправить коррелированный ответ.

## Входящее событие от analytics-service

Topic: `analytics.skill_assessment.updated`.

Пример:

```json
{
  "event_id": "11111111-1111-1111-1111-111111111111",
  "event_version": 1,
  "event_type": "analytics.skill_assessment.updated",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "source_task_id": "00000000-0000-0000-0000-000000010004",
  "source_attempt_id": "33333333-3333-3333-3333-333333333333",
  "observed_until": "2026-05-11T12:00:00Z",
  "assessment_mode": "merge",
  "analysis": {
    "analysis_run_id": "run-001",
    "model_version": "llama-3-8b-sql-lora-v1",
    "prompt_version": "mastery-v2",
    "seed": 42,
    "temperature": 0.0
  },
  "graph_code": "DA",
  "professional_track": "DA",
  "recommendation_reason": "Пользователь показывает сильные результаты в агрегациях",
  "skill_scores": [
    {
      "skill_code": "group_by",
      "mastery_score": 0.82,
      "confidence": 0.75,
      "components": {
        "correctness": 0.9,
        "independence": 0.7,
        "efficiency": 0.85
      },
      "reason": "Корректная группировка, но использована подсказка"
    }
  ],
  "recommended_skills": [
    {
      "skill_code": "having",
      "priority": 0.91,
      "recommended_action": "practice",
      "reason": "Следующий слабый тег после GROUP BY"
    }
  ],
  "user_graph_overlay": {
    "graph_code": "DA",
    "skills": [
      {
        "skill_code": "having",
        "priority_weight": 1.35,
        "mastery_threshold": 0.82,
        "reason": "Усилить HAVING для аналитического трека"
      }
    ],
    "dependencies": [
      {
        "skill_code": "having",
        "depends_on_skill_code": "group_by",
        "required_mastery": 0.78
      }
    ]
  },
  "created_at": "2026-05-11T12:00:15Z"
}
```

После такого события сервис:

1. идемпотентно помечает event;
2. применяет skill assessment с защитой по `observed_until`;
3. заменяет предыдущий snapshot `recommended_skills` для текущего `user_id + graph_id`;
4. применяет user graph overlay только для данного пользователя;
5. снимает `analysis_pending`, если `source_task_id` и `source_attempt_id` совпадают с текущим pending-состоянием.

## assessment_mode

Поддерживаются режимы:

```text
replace — заменить mastery_score оценкой LLM;
delta   — применить только delta;
merge   — объединить текущую оценку с LLM-оценкой по confidence.
```

По умолчанию используется `merge`.

## Выбор следующей задачи

`GET /tasks/next` теперь:

1. проверяет `user_analysis_state`;
2. если есть active pending-state, возвращает `409 analysis_pending` с контекстом ожидания;
3. если ожидание истекло, pending-state автоматически переводится в `expired`, и сервис продолжает выбор задачи по текущей модели;
4. загружает active reference graph;
5. накладывает user-specific overrides;
6. учитывает последний snapshot `recommended_skills` от LLM/analytics;
7. учитывает Эббингауза через `effective_mastery`;
8. выбирает новую задачу;
9. если новых задач нет, выбирает задачу на повторение;
10. если и таких нет, выбирает наиболее подходящую уже решенную задачу.


### Ответ `409 analysis_pending`

Когда analytics/LLM еще не прислал оценку и рекомендации после успешного выполнения задачи, `GET /tasks/next` возвращает:

```json
{
  "error": "analysis_pending",
  "pending_task_id": "8e65d38e-0a3d-4f42-8848-5a5f6e1b4a1f",
  "pending_attempt_id": "1a36baf2-2c02-4c62-90ae-47e6f7e12653",
  "pending_since": "2026-05-11T12:00:03Z",
  "wait_until": "2026-05-11T12:02:03Z",
  "retry_after_seconds": 5,
  "analysis_state": {
    "user_id": "...",
    "pending_task_id": "8e65d38e-0a3d-4f42-8848-5a5f6e1b4a1f",
    "pending_attempt_id": "1a36baf2-2c02-4c62-90ae-47e6f7e12653",
    "status": "pending",
    "pending_since": "2026-05-11T12:00:03Z",
    "wait_until": "2026-05-11T12:02:03Z"
  }
}
```

Также выставляется HTTP-заголовок `Retry-After`. Клиент/API Gateway должен повторить `GET /tasks/next` через указанное время. Сервер не держит исходный HTTP-запрос открытым и не делает автоматический retry за клиента.

При `soft` timeout после `wait_until` выбор задачи разблокируется. Если analytics/LLM пришлет ответ позже, он будет применен только при прохождении защиты от устаревших оценок: `observed_until` должен быть не старее уже примененной оценки по соответствующему skill.

## Проверка локально

```bash
go mod tidy
go test ./...
```

В изолированной песочнице зависимости `pgx` и `kafka-go` могут не скачаться без доступа к `proxy.golang.org`.

## Подсказки

`task-progress` exposes `POST /tasks/{task_id}/hint` as the public endpoint for requesting a hint.
The endpoint is available after the first submit for the task, including already solved tasks. This allows a user to request a hint before retrying a completed task to improve the score.
The service does not generate the hint itself. It calls `analytics-service` through `ANALYTICS_INTERNAL_URL`:

```http
POST /internal/hints/generate
```

Generation is bounded by `HINT_GENERATION_TIMEOUT` (`15s` by default). If analytics/LLM does not respond in time, the endpoint returns:

```json
{"error":"hint_generation_failed"}
```

Failed hint generations are not written to `user_task_hint_state`, so they do not affect `hint_used`, `hint_count` or the independence score in the downstream LLM analysis.

Successful hints are stored only as compact state in `user_task_hint_state`: count, last hint id, type and attempt number. Full hint text, prompt/context and LLM response are stored in ClickHouse by `analytics-service`.

On later submits, `task.checked` and `task.completed` are enriched from `user_task_hint_state`. The client/playground-provided hint flags are not trusted as the source of truth.
