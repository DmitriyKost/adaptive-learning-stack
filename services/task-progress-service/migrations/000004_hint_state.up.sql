CREATE TABLE IF NOT EXISTS user_task_hint_state (
    user_id UUID NOT NULL,
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    hint_count INT NOT NULL DEFAULT 0 CHECK (hint_count >= 0),
    last_hint_id UUID,
    last_hint_type TEXT,
    last_hint_attempt_number INT CHECK (last_hint_attempt_number IS NULL OR last_hint_attempt_number > 0),
    last_hint_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_user_task_hint_state_user_task ON user_task_hint_state(user_id, task_id);
