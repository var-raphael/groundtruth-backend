package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
)

func CreateJob(ctx context.Context, pool *pgxpool.Pool, j *models.Job) (*models.Job, error) {
	const q = `
		INSERT INTO jobs (recruiter_id, title, description, stack, location_mode,
			location_countries, min_years_experience, candidate_limit, timezone, min_overlap_hours)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`

	row := pool.QueryRow(ctx, q,
		j.RecruiterID, j.Title, j.Description, j.Stack, j.LocationMode,
		j.LocationCountries, j.MinYearsExperience, j.CandidateLimit, j.Timezone, j.MinOverlapHours,
	)
	if err := row.Scan(&j.ID, &j.CreatedAt); err != nil {
		return nil, fmt.Errorf("inserting job: %w", err)
	}
	return j, nil
}

func GetJob(ctx context.Context, pool *pgxpool.Pool, id string) (*models.Job, error) {
	const q = `
		SELECT id, recruiter_id, title, description, stack, location_mode,
			location_countries, min_years_experience, candidate_limit, timezone, min_overlap_hours, created_at
		FROM jobs WHERE id = $1`

	var j models.Job
	err := pool.QueryRow(ctx, q, id).Scan(
		&j.ID, &j.RecruiterID, &j.Title, &j.Description, &j.Stack, &j.LocationMode,
		&j.LocationCountries, &j.MinYearsExperience, &j.CandidateLimit, &j.Timezone, &j.MinOverlapHours, &j.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching job %s: %w", id, err)
	}
	return &j, nil
}

func ListJobsByRecruiter(ctx context.Context, pool *pgxpool.Pool, recruiterID string) ([]models.Job, error) {
	const q = `
		SELECT id, recruiter_id, title, description, stack, location_mode,
			location_countries, min_years_experience, candidate_limit, timezone, min_overlap_hours, created_at
		FROM jobs WHERE recruiter_id = $1
		ORDER BY created_at DESC`

	rows, err := pool.Query(ctx, q, recruiterID)
	if err != nil {
		return nil, fmt.Errorf("listing jobs for recruiter %s: %w", recruiterID, err)
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var j models.Job
		if err := rows.Scan(
			&j.ID, &j.RecruiterID, &j.Title, &j.Description, &j.Stack, &j.LocationMode,
			&j.LocationCountries, &j.MinYearsExperience, &j.CandidateLimit, &j.Timezone, &j.MinOverlapHours, &j.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning job row: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func DeleteJob(ctx context.Context, pool *pgxpool.Pool, id string) error {
	const q = `DELETE FROM jobs WHERE id = $1`
	_, err := pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting job %s: %w", id, err)
	}
	return nil
}
