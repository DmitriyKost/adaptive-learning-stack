CREATE TABLE IF NOT EXISTS user_graph_skill_overrides (
    user_id UUID NOT NULL,
    graph_id UUID NOT NULL REFERENCES learning_graphs(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    position INT,
    is_required BOOLEAN,
    priority_weight NUMERIC(6, 3) CHECK (priority_weight IS NULL OR priority_weight >= 0),
    mastery_threshold NUMERIC(6, 4) CHECK (mastery_threshold IS NULL OR (mastery_threshold > 0 AND mastery_threshold <= 1)),
    reason TEXT,
    source_event_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, graph_id, skill_id)
);

CREATE TABLE IF NOT EXISTS user_skill_dependency_overrides (
    user_id UUID NOT NULL,
    graph_id UUID NOT NULL REFERENCES learning_graphs(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    depends_on_skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    strength NUMERIC(6, 3) CHECK (strength IS NULL OR strength > 0),
    required_mastery NUMERIC(6, 4) CHECK (required_mastery IS NULL OR (required_mastery > 0 AND required_mastery <= 1)),
    reason TEXT,
    source_event_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, graph_id, skill_id, depends_on_skill_id),
    CHECK (skill_id <> depends_on_skill_id)
);

CREATE TABLE IF NOT EXISTS user_skill_recommendations (
    user_id UUID NOT NULL,
    graph_id UUID NOT NULL REFERENCES learning_graphs(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    priority NUMERIC(6, 4) NOT NULL CHECK (priority >= 0 AND priority <= 1),
    recommended_action TEXT,
    reason TEXT,
    source_event_id UUID,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, graph_id, skill_id)
);

CREATE TABLE IF NOT EXISTS user_analysis_state (
    user_id UUID PRIMARY KEY,
    pending_task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    pending_attempt_id UUID,
    status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'expired', 'failed')),
    pending_since TIMESTAMPTZ NOT NULL DEFAULT now(),
    wait_until TIMESTAMPTZ,
    source_event_id UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_skill_assessment_versions (
    user_id UUID NOT NULL,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    last_observed_until TIMESTAMPTZ NOT NULL,
    last_analysis_run_id TEXT,
    model_version TEXT,
    prompt_version TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_user_skill_recommendations_user_graph ON user_skill_recommendations(user_id, graph_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_user_analysis_state_pending ON user_analysis_state(user_id, status, wait_until);
