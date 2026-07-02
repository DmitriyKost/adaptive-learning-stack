CREATE TABLE IF NOT EXISTS user_next_task_recommendations (
    user_id UUID PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('pending', 'ready', 'failed')),
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    graph_id UUID REFERENCES learning_graphs(id) ON DELETE SET NULL,
    reason TEXT,
    score NUMERIC(12, 6) NOT NULL DEFAULT 0,
    graph_code TEXT,
    professional_track TEXT,
    repeat_mode BOOLEAN NOT NULL DEFAULT false,
    recommended_skills JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_event_id UUID,
    source_attempt_id UUID,
    source_analysis_run_id TEXT,
    wait_until TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_next_task_status ON user_next_task_recommendations(status, wait_until, expires_at);
