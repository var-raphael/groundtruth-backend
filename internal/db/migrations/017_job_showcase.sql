ALTER TABLE jobs ADD COLUMN showcase BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX idx_jobs_showcase ON jobs(showcase) WHERE showcase;
