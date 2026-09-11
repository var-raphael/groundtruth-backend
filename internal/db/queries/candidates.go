package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
)

func CreateCandidate(ctx context.Context, pool *pgxpool.Pool, c *models.Candidate) (*models.Candidate, error) {
	const q = `
		INSERT INTO candidates (job_id, full_name, email, country, city, timezone,
			years_experience, github_id, github_username, github_token, linkedin, x, portfolio, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, applied_at, status_updated_at`

	if c.Status == "" {
		c.Status = models.StatusQueued
	}

	row := pool.QueryRow(ctx, q,
		c.JobID, c.FullName, c.Email, c.Country, c.City, c.Timezone,
		c.YearsExperience, c.GithubID, c.GithubUsername, c.GithubToken,
		c.LinkedIn, c.X, c.Portfolio, c.Status,
	)
	if err := row.Scan(&c.ID, &c.AppliedAt, &c.StatusUpdatedAt); err != nil {
		return nil, fmt.Errorf("inserting candidate: %w", err)
	}
	return c, nil
}

func GetCandidate(ctx context.Context, pool *pgxpool.Pool, id string) (*models.Candidate, error) {
	const q = `
		SELECT id, job_id, full_name, email, country, city, timezone, years_experience,
			github_id, github_username, github_token, linkedin, x, portfolio, status, status_updated_at, applied_at
		FROM candidates WHERE id = $1`

	var c models.Candidate
	err := pool.QueryRow(ctx, q, id).Scan(
		&c.ID, &c.JobID, &c.FullName, &c.Email, &c.Country, &c.City, &c.Timezone, &c.YearsExperience,
		&c.GithubID, &c.GithubUsername, &c.GithubToken, &c.LinkedIn, &c.X, &c.Portfolio, &c.Status, &c.StatusUpdatedAt, &c.AppliedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching candidate %s: %w", id, err)
	}
	return &c, nil
}

func ListCandidatesByJob(ctx context.Context, pool *pgxpool.Pool, jobID string, status models.CandidateStatus) ([]models.Candidate, error) {
	q := `
		SELECT id, job_id, full_name, email, country, city, timezone, years_experience,
			github_id, github_username, github_token, linkedin, x, portfolio, status, status_updated_at, applied_at
		FROM candidates WHERE job_id = $1`
	args := []any{jobID}

	if status != "" {
		q += ` AND status = $2`
		args = append(args, status)
	}
	q += ` ORDER BY applied_at DESC`

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing candidates for job %s: %w", jobID, err)
	}
	defer rows.Close()

	var candidates []models.Candidate
	for rows.Next() {
		var c models.Candidate
		if err := rows.Scan(
			&c.ID, &c.JobID, &c.FullName, &c.Email, &c.Country, &c.City, &c.Timezone, &c.YearsExperience,
			&c.GithubID, &c.GithubUsername, &c.GithubToken, &c.LinkedIn, &c.X, &c.Portfolio, &c.Status, &c.StatusUpdatedAt, &c.AppliedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning candidate row: %w", err)
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func UpdateCandidateStatus(ctx context.Context, pool *pgxpool.Pool, id string, status models.CandidateStatus) error {
	const q = `UPDATE candidates SET status = $2, status_updated_at = now() WHERE id = $1`
	_, err := pool.Exec(ctx, q, id, status)
	if err != nil {
		return fmt.Errorf("updating status for candidate %s: %w", id, err)
	}
	return nil
}

func CountCandidatesForJob(ctx context.Context, pool *pgxpool.Pool, jobID string) (int, error) {
	const q = `SELECT count(*) FROM candidates WHERE job_id = $1`
	var count int
	if err := pool.QueryRow(ctx, q, jobID).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting candidates for job %s: %w", jobID, err)
	}
	return count, nil
}

func ResetStaleScoringCandidates(ctx context.Context, pool *pgxpool.Pool, staleAfterMinutes int) (int, error) {
	const q = `
		UPDATE candidates
		SET status = $1, status_updated_at = now()
		WHERE status = $2
		AND status_updated_at < now() - make_interval(mins => $3)`

	tag, err := pool.Exec(ctx, q, models.StatusQueued, models.StatusScoring, staleAfterMinutes)
	if err != nil {
		return 0, fmt.Errorf("resetting stale scoring candidates: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
