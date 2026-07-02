#!/usr/bin/env bash
set -euo pipefail

TASK_PROGRESS_URL="${TASK_PROGRESS_URL:-http://localhost:8083}"
PG_CONTAINER="${PG_CONTAINER:-adaptive-task-progress-postgres}"
PG_USER="${PG_USER:-task_progress}"
PG_DB="${PG_DB:-task_progress}"

USER_A="11111111-1111-1111-1111-111111111111"
USER_B="22222222-2222-2222-2222-222222222222"

log() { printf '\n==> %s\n' "$*"; }

log "Preparing two synthetic learners in core_sql graph"
docker exec -i "$PG_CONTAINER" psql -U "$PG_USER" -d "$PG_DB" -v ON_ERROR_STOP=1 <<SQL
WITH users(user_id) AS (
  VALUES
    ('$USER_A'::uuid),
    ('$USER_B'::uuid)
)
DELETE FROM user_next_task_recommendations WHERE user_id IN (SELECT user_id FROM users);

WITH users(user_id) AS (
  VALUES
    ('$USER_A'::uuid),
    ('$USER_B'::uuid)
)
DELETE FROM user_analysis_state WHERE user_id IN (SELECT user_id FROM users);

WITH users(user_id) AS (
  VALUES
    ('$USER_A'::uuid),
    ('$USER_B'::uuid)
)
DELETE FROM user_skill_recommendations WHERE user_id IN (SELECT user_id FROM users);

WITH users(user_id) AS (
  VALUES
    ('$USER_A'::uuid),
    ('$USER_B'::uuid)
)
DELETE FROM user_task_status WHERE user_id IN (SELECT user_id FROM users);

WITH users(user_id) AS (
  VALUES
    ('$USER_A'::uuid),
    ('$USER_B'::uuid)
)
DELETE FROM user_skills WHERE user_id IN (SELECT user_id FROM users);

WITH users(user_id) AS (
  VALUES
    ('$USER_A'::uuid),
    ('$USER_B'::uuid)
)
DELETE FROM user_learning_profiles WHERE user_id IN (SELECT user_id FROM users);

INSERT INTO user_learning_profiles (user_id, graph_id, professional_track, recommendation_reason, updated_at)
SELECT '$USER_A'::uuid, id, 'core', 'demo: weak WHERE', now()
FROM learning_graphs WHERE code = 'core_sql'
ON CONFLICT (user_id) DO UPDATE SET graph_id = EXCLUDED.graph_id, professional_track = EXCLUDED.professional_track, recommendation_reason = EXCLUDED.recommendation_reason, updated_at = now();

INSERT INTO user_learning_profiles (user_id, graph_id, professional_track, recommendation_reason, updated_at)
SELECT '$USER_B'::uuid, id, 'core', 'demo: weak INNER JOIN', now()
FROM learning_graphs WHERE code = 'core_sql'
ON CONFLICT (user_id) DO UPDATE SET graph_id = EXCLUDED.graph_id, professional_track = EXCLUDED.professional_track, recommendation_reason = EXCLUDED.recommendation_reason, updated_at = now();

-- Student A: all core_sql skills are high, but WHERE is weak.
INSERT INTO user_skills (user_id, skill_id, mastery_score, confidence, attempts_count, success_count, last_used_at, updated_at)
SELECT '$USER_A'::uuid,
       s.id,
       CASE WHEN s.code = 'where' THEN 0.20 ELSE 0.85 END,
       0.90,
       CASE WHEN s.code = 'where' THEN 1 ELSE 5 END,
       CASE WHEN s.code = 'where' THEN 0 ELSE 5 END,
       now(),
       now()
FROM graph_skills gs
JOIN learning_graphs g ON g.id = gs.graph_id AND g.code = 'core_sql'
JOIN skills s ON s.id = gs.skill_id
ON CONFLICT (user_id, skill_id) DO UPDATE SET
  mastery_score = EXCLUDED.mastery_score,
  confidence = EXCLUDED.confidence,
  attempts_count = EXCLUDED.attempts_count,
  success_count = EXCLUDED.success_count,
  last_used_at = EXCLUDED.last_used_at,
  updated_at = EXCLUDED.updated_at;

-- Student B: all core_sql skills are high, but INNER JOIN is weak.
INSERT INTO user_skills (user_id, skill_id, mastery_score, confidence, attempts_count, success_count, last_used_at, updated_at)
SELECT '$USER_B'::uuid,
       s.id,
       CASE WHEN s.code = 'inner_join' THEN 0.20 ELSE 0.85 END,
       0.90,
       CASE WHEN s.code = 'inner_join' THEN 1 ELSE 5 END,
       CASE WHEN s.code = 'inner_join' THEN 0 ELSE 5 END,
       now(),
       now()
FROM graph_skills gs
JOIN learning_graphs g ON g.id = gs.graph_id AND g.code = 'core_sql'
JOIN skills s ON s.id = gs.skill_id
ON CONFLICT (user_id, skill_id) DO UPDATE SET
  mastery_score = EXCLUDED.mastery_score,
  confidence = EXCLUDED.confidence,
  attempts_count = EXCLUDED.attempts_count,
  success_count = EXCLUDED.success_count,
  last_used_at = EXCLUDED.last_used_at,
  updated_at = EXCLUDED.updated_at;
SQL

log "Student A: weak WHERE"
curl -s \
  -H "X-User-ID: $USER_A" \
  -H "X-User-Role: student" \
  "$TASK_PROGRESS_URL/tasks/next" | jq '{task_id: .task.id, title: .task.title, difficulty: .task.difficulty, skills: [.task.skills[].skill_code], score: .score, reason: .reason}'

log "Student B: weak INNER JOIN"
curl -s \
  -H "X-User-ID: $USER_B" \
  -H "X-User-Role: student" \
  "$TASK_PROGRESS_URL/tasks/next" | jq '{task_id: .task.id, title: .task.title, difficulty: .task.difficulty, skills: [.task.skills[].skill_code], score: .score, reason: .reason}'
