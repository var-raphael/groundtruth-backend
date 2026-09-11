-- 001_init.sql
-- Core tables: recruiters, jobs, candidates

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- for gen_random_uuid()

CREATE TABLE recruiters (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    google_id     TEXT NOT NULL UNIQUE, -- Google OAuth 'sub' claim, the stable subject ID
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    plan          TEXT NOT NULL DEFAULT 'free', -- drives candidate_limit on jobs
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE jobs (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recruiter_id         UUID NOT NULL REFERENCES recruiters(id) ON DELETE CASCADE,
    title                TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    stack                TEXT[] NOT NULL DEFAULT '{}',

    location_mode        TEXT NOT NULL DEFAULT 'anywhere', -- anywhere | country | onsite
    location_countries   TEXT[] NOT NULL DEFAULT '{}',

    min_years_experience INTEGER NOT NULL DEFAULT 0,

    timezone             TEXT NOT NULL,
    min_overlap_hours    INTEGER NOT NULL DEFAULT 0,
    candidate_limit      INTEGER NOT NULL DEFAULT 50, -- plan-driven, set server-side

    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT location_mode_valid CHECK (location_mode IN ('anywhere', 'country', 'onsite'))
);

CREATE INDEX idx_jobs_recruiter_id ON jobs(recruiter_id);

CREATE TABLE candidates (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,

    full_name         TEXT NOT NULL,
    email             TEXT NOT NULL,
    country           TEXT NOT NULL DEFAULT '',
    city              TEXT NOT NULL DEFAULT '',
    timezone          TEXT NOT NULL DEFAULT '',
    years_experience  INTEGER NOT NULL DEFAULT 0,

    github_id         BIGINT NOT NULL, -- GitHub's stable numeric user ID from OAuth
    github_username   TEXT NOT NULL,   -- display/lookup handle, can change over time
    github_token      TEXT, -- encrypted at rest; never sent back to clients

    linkedin          TEXT,
    x                 TEXT,
    portfolio         TEXT,

    status            TEXT NOT NULL DEFAULT 'queued', -- queued | scoring | scored | failed

    applied_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT status_valid CHECK (status IN ('queued', 'scoring', 'scored', 'failed'))
);

CREATE INDEX idx_candidates_job_id ON candidates(job_id);
CREATE INDEX idx_candidates_status ON candidates(status);
CREATE UNIQUE INDEX idx_candidates_job_github ON candidates(job_id, github_id);
