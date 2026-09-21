CREATE TABLE usage_events (
    id           BIGSERIAL PRIMARY KEY,
    recruiter_id UUID NOT NULL REFERENCES recruiters(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    action       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT usage_action_valid CHECK (action IN ('outreach', 'rescan'))
);

CREATE INDEX idx_usage_candidate_time ON usage_events(candidate_id, created_at DESC);
CREATE INDEX idx_usage_recruiter_action ON usage_events(recruiter_id, action, candidate_id);
