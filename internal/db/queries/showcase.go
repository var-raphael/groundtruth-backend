package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
)

type ShowcaseJob struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	Stack              []string            `json:"stack"`
	LocationMode       models.LocationMode `json:"location_mode"`
	LocationCountries  []string            `json:"location_countries"`
	MinYearsExperience int                 `json:"min_years_experience"`
	CandidateCount     int                 `json:"candidate_count"`
	ScoredCount        int                 `json:"scored_count"`
	Status             string              `json:"status"`
	CreatedAt          time.Time           `json:"created_at"`
}

const showcaseSelect = `
	SELECT j.id, j.title, j.description, j.stack, j.location_mode, j.location_countries,
		j.min_years_experience, j.created_at,
		count(c.id),
		count(c.id) FILTER (WHERE c.status = 'scored'),
		count(c.id) FILTER (WHERE c.status IN ('queued', 'extracting', 'extracted', 'scoring'))
	FROM jobs j
	LEFT JOIN candidates c ON c.job_id = j.id
	WHERE j.showcase`

func scanShowcaseJob(row pgx.Row) (*ShowcaseJob, error) {
	var j ShowcaseJob
	var active int
	err := row.Scan(
		&j.ID, &j.Title, &j.Description, &j.Stack, &j.LocationMode, &j.LocationCountries,
		&j.MinYearsExperience, &j.CreatedAt, &j.CandidateCount, &j.ScoredCount, &active,
	)
	if err != nil {
		return nil, err
	}
	switch {
	case j.CandidateCount == 0:
		j.Status = "draft"
	case active > 0:
		j.Status = "scanning"
	default:
		j.Status = "ready"
	}
	if j.Stack == nil {
		j.Stack = []string{}
	}
	if j.LocationCountries == nil {
		j.LocationCountries = []string{}
	}
	return &j, nil
}

func SetJobShowcase(ctx context.Context, pool *pgxpool.Pool, jobID string) error {
	if _, err := pool.Exec(ctx, `UPDATE jobs SET showcase = true WHERE id = $1`, jobID); err != nil {
		return fmt.Errorf("marking job %s as showcase: %w", jobID, err)
	}
	return nil
}

func IsShowcaseJob(ctx context.Context, pool *pgxpool.Pool, jobID string) (bool, error) {
	var ok bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM jobs WHERE id = $1 AND showcase)`, jobID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("checking showcase job %s: %w", jobID, err)
	}
	return ok, nil
}

func ListShowcaseJobs(ctx context.Context, pool *pgxpool.Pool) ([]ShowcaseJob, error) {
	rows, err := pool.Query(ctx, showcaseSelect+` GROUP BY j.id ORDER BY j.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing showcase jobs: %w", err)
	}
	defer rows.Close()

	jobs := []ShowcaseJob{}
	for rows.Next() {
		j, err := scanShowcaseJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning showcase job: %w", err)
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

func GetShowcaseJob(ctx context.Context, pool *pgxpool.Pool, id string) (*ShowcaseJob, error) {
	j, err := scanShowcaseJob(pool.QueryRow(ctx, showcaseSelect+` AND j.id = $1 GROUP BY j.id`, id))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching showcase job %s: %w", id, err)
	}
	return j, nil
}
