CREATE TABLE candidate_identities (
    auth_user_id    UUID PRIMARY KEY,
    github_id       BIGINT NOT NULL,
    github_username TEXT NOT NULL,
    github_token    TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_candidate_identities_github ON candidate_identities(github_id);
