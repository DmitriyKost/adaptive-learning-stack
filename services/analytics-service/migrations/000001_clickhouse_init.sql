CREATE TABLE IF NOT EXISTS raw_events (
    event_id String,
    event_type LowCardinality(String),
    user_id String,
    task_id String,
    attempt_id String,
    source_topic LowCardinality(String),
    kafka_partition Int32,
    kafka_offset Int64,
    event_time DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC'),
    payload String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(event_time)
ORDER BY (event_type, user_id, event_time, event_id);

CREATE TABLE IF NOT EXISTS task_attempt_logs (
    event_id String,
    user_id String,
    task_id String,
    attempt_id String,
    attempt_number Int32,
    execution_success Bool,
    is_correct Bool,
    submitted_sql String,
    reference_sql String,
    error_type String,
    error_message String,
    execution_time_ms Int64,
    row_count Int32,
    hint_requested Bool,
    hint_used Bool,
    hint_type String,
    hints_count Int32,
    skills Array(String),
    task_json String,
    execution_json String,
    actual_result_json String,
    expected_result_json String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, task_id, created_at, attempt_number, event_id);

CREATE TABLE IF NOT EXISTS task_completion_logs (
    event_id String,
    user_id String,
    task_id String,
    attempt_id String,
    graph_code LowCardinality(String),
    professional_track LowCardinality(String),
    graph_state_json String,
    task_json String,
    execution_json String,
    hint_json String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, created_at, task_id, event_id);

CREATE TABLE IF NOT EXISTS task_recommendation_logs (
    event_id String,
    user_id String,
    task_id String,
    graph_code LowCardinality(String),
    score Float64,
    reason String,
    repeat_mode Bool,
    candidate_source LowCardinality(String),
    payload String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, created_at, task_id, event_id);

CREATE TABLE IF NOT EXISTS learner_model_update_logs (
    event_id String,
    user_id String,
    skill_id String,
    skill_code LowCardinality(String),
    old_mastery_score Float64,
    new_mastery_score Float64,
    old_confidence Float64,
    new_confidence Float64,
    source LowCardinality(String),
    reason String,
    analysis_run_id String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, skill_code, created_at, event_id);

CREATE TABLE IF NOT EXISTS llm_analysis_runs (
    run_id String,
    user_id String,
    source_task_id String,
    source_attempt_id String,
    status LowCardinality(String),
    model_version String,
    prompt_version String,
    request_context String,
    response_payload String,
    error_message String,
    started_at DateTime64(3, 'UTC'),
    completed_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(started_at)
ORDER BY (user_id, started_at, run_id);

CREATE TABLE IF NOT EXISTS skill_assessment_logs (
    analysis_run_id String,
    user_id String,
    skill_id String,
    skill_code LowCardinality(String),
    mastery_score Float64,
    confidence Float64,
    correctness Float64,
    independence Float64,
    efficiency Float64,
    reason String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, skill_code, created_at, analysis_run_id);

CREATE TABLE IF NOT EXISTS skill_recommendation_logs (
    analysis_run_id String,
    user_id String,
    skill_id String,
    skill_code LowCardinality(String),
    priority Float64,
    recommended_action LowCardinality(String),
    reason String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, priority, created_at, analysis_run_id);

CREATE TABLE IF NOT EXISTS career_recommendation_logs (
    event_id String,
    user_id String,
    analysis_run_id String,
    primary_track LowCardinality(String),
    primary_title String,
    score Float64,
    recommended_graph_code LowCardinality(String),
    strong_skills Array(String),
    weak_skills Array(String),
    explanation String,
    payload String,
    created_at DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (user_id, created_at, event_id);

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
