CREATE TABLE share_link_outreach_events (
    id            BIGSERIAL PRIMARY KEY,
    share_token   UUID NOT NULL REFERENCES job_share_links(token) ON DELETE CASCADE,
    candidate_id  UUID NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_share_link_outreach_token ON share_link_outreach_events(share_token);
