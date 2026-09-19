ALTER TABLE candidates DROP CONSTRAINT status_valid;
ALTER TABLE candidates ADD CONSTRAINT status_valid CHECK (status IN ('queued', 'extracting', 'extracted', 'scoring', 'scored', 'failed'));

CREATE TABLE candidate_evidence (
    candidate_id UUID PRIMARY KEY REFERENCES candidates(id) ON DELETE CASCADE,
    top_repos JSONB NOT NULL,
    contributions JSONB NOT NULL,
    contributions_error TEXT,
    extracted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
