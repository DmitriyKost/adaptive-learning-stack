CREATE TABLE IF NOT EXISTS hint_events (
    event_id String,
    user_id String,
    task_id String,
    hint_id String,
    event_type LowCardinality(String),
    hint_type LowCardinality(String),
    attempt_number Int32,
    related_skills Array(String),
    message String,
    request_json String,
    response_json String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, task_id, created_at, hint_id);
