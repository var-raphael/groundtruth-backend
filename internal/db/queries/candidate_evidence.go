package queries

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/scoring"
)

func SaveCandidateEvidence(ctx context.Context, pool *pgxpool.Pool, candidateID string, evidence *scoring.GithubEvidence) error {
	topRepos, err := json.Marshal(evidence.TopRepos)
	if err != nil {
		return fmt.Errorf("marshaling top repos for candidate %s: %w", candidateID, err)
	}
	contributions, err := json.Marshal(evidence.Contributions)
	if err != nil {
		return fmt.Errorf("marshaling contributions for candidate %s: %w", candidateID, err)
	}

	const q = `
		INSERT INTO candidate_evidence (candidate_id, top_repos, contributions, contributions_error, extracted_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (candidate_id) DO UPDATE SET
			top_repos = EXCLUDED.top_repos,
			contributions = EXCLUDED.contributions,
			contributions_error = EXCLUDED.contributions_error,
			extracted_at = now()`

	_, err = pool.Exec(ctx, q, candidateID, topRepos, contributions, nullableString(evidence.ContributionsError))
	if err != nil {
		return fmt.Errorf("saving github evidence for candidate %s: %w", candidateID, err)
	}
	return nil
}

func GetCandidateEvidence(ctx context.Context, pool *pgxpool.Pool, candidateID string) (*scoring.GithubEvidence, error) {
	const q = `SELECT top_repos, contributions, contributions_error FROM candidate_evidence WHERE candidate_id = $1`

	var topRepos, contributions []byte
	var contributionsError *string
	err := pool.QueryRow(ctx, q, candidateID).Scan(&topRepos, &contributions, &contributionsError)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching github evidence for candidate %s: %w", candidateID, err)
	}

	var evidence scoring.GithubEvidence
	if err := json.Unmarshal(topRepos, &evidence.TopRepos); err != nil {
		return nil, fmt.Errorf("unmarshaling top repos for candidate %s: %w", candidateID, err)
	}
	if err := json.Unmarshal(contributions, &evidence.Contributions); err != nil {
		return nil, fmt.Errorf("unmarshaling contributions for candidate %s: %w", candidateID, err)
	}
	if contributionsError != nil {
		evidence.ContributionsError = *contributionsError
	}
	return &evidence, nil
}

func DeleteCandidateEvidence(ctx context.Context, pool *pgxpool.Pool, candidateID string) error {
	const q = `DELETE FROM candidate_evidence WHERE candidate_id = $1`
	_, err := pool.Exec(ctx, q, candidateID)
	if err != nil {
		return fmt.Errorf("deleting github evidence for candidate %s: %w", candidateID, err)
	}
	return nil
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
