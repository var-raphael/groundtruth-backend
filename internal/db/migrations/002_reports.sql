-- 002_reports.sql
-- Stores the generated CandidateReport (evidence + reasoning) per (candidate, job).
-- Kept as JSONB rather than fully normalized: report shape is still evolving
-- with the LLM prompt/schema, and it's always read/written as a whole document
-- (never queried piecemeal), so JSONB avoids constant migrations as fields change.

CREATE TABLE candidate_reports (
    candidate_id  UUID NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    job_id        UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,

    score         NUMERIC(4,2) NOT NULL, -- reasoning.score, surfaced for sorting/filtering
    stack_match   TEXT NOT NULL,         -- "strong" | "partial" | "weak"
    has_trust_flag BOOLEAN NOT NULL DEFAULT false,

    report        JSONB NOT NULL, -- full models.CandidateReport, camelCase as serialized

    generated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (candidate_id, job_id)
);

CREATE INDEX idx_candidate_reports_job_id ON candidate_reports(job_id);
CREATE INDEX idx_candidate_reports_score ON candidate_reports(job_id, score DESC);
