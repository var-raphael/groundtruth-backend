package worker

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/plans"
)

func syncJobLimits(ctx context.Context, pool *pgxpool.Pool) error {
	for _, name := range []string{plans.Free, plans.Pro, plans.Internal} {
		limit := plans.For(name).MaxCandidatesPerJob
		if limit == plans.Unlimited {
			continue
		}
		const q = `
			UPDATE jobs SET candidate_limit = $1
			WHERE candidate_limit <> $1
			AND recruiter_id IN (SELECT id FROM recruiters WHERE plan = $2)`
		if _, err := pool.Exec(ctx, q, limit, name); err != nil {
			return fmt.Errorf("syncing job limits for plan %s: %w", name, err)
		}
	}
	return nil
}

func PromoteUnscanned(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if err := syncJobLimits(ctx, pool); err != nil {
		return 0, err
	}

	rows, err := pool.Query(ctx, `
		SELECT DISTINCT j.id, j.candidate_limit
		FROM candidates c
		JOIN jobs j ON j.id = c.job_id
		WHERE c.status = $1`, models.StatusUnscanned)
	if err != nil {
		return 0, fmt.Errorf("listing jobs with unscanned candidates: %w", err)
	}
	type jobLimit struct {
		id    string
		limit int
	}
	var jobs []jobLimit
	for rows.Next() {
		var jl jobLimit
		if err := rows.Scan(&jl.id, &jl.limit); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning job limit row: %w", err)
		}
		jobs = append(jobs, jl)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	released := 0
	for _, jl := range jobs {
		var used int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM candidates WHERE job_id = $1 AND status != $2`,
			jl.id, models.StatusUnscanned).Scan(&used)
		if err != nil {
			return released, fmt.Errorf("counting active candidates for job %s: %w", jl.id, err)
		}
		room := jl.limit - used
		if room <= 0 {
			continue
		}
		tag, err := pool.Exec(ctx, `
			UPDATE candidates SET status = $1, status_updated_at = now()
			WHERE id IN (
				SELECT id FROM candidates
				WHERE job_id = $2 AND status = $3
				ORDER BY applied_at ASC
				LIMIT $4
			)`, models.StatusQueued, jl.id, models.StatusUnscanned, room)
		if err != nil {
			return released, fmt.Errorf("releasing unscanned candidates for job %s: %w", jl.id, err)
		}
		released += int(tag.RowsAffected())
	}
	return released, nil
}
