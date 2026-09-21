package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RecordUsage(ctx context.Context, pool *pgxpool.Pool, recruiterID, candidateID, action string) error {
	const q = `INSERT INTO usage_events (recruiter_id, candidate_id, action) VALUES ($1, $2, $3)`

	_, err := pool.Exec(ctx, q, recruiterID, candidateID, action)
	if err != nil {
		return fmt.Errorf("recording %s usage for candidate %s: %w", action, candidateID, err)
	}
	return nil
}

func LastUsageAt(ctx context.Context, pool *pgxpool.Pool, candidateID string) (*time.Time, error) {
	const q = `SELECT max(created_at) FROM usage_events WHERE candidate_id = $1`

	var last *time.Time
	if err := pool.QueryRow(ctx, q, candidateID).Scan(&last); err != nil {
		return nil, fmt.Errorf("fetching last usage for candidate %s: %w", candidateID, err)
	}
	return last, nil
}

func CountUsageSince(ctx context.Context, pool *pgxpool.Pool, candidateID, action string, since time.Time) (int, time.Time, error) {
	const q = `SELECT count(*), coalesce(min(created_at), now()) FROM usage_events WHERE candidate_id = $1 AND action = $2 AND created_at >= $3`

	var count int
	var oldest time.Time
	if err := pool.QueryRow(ctx, q, candidateID, action, since).Scan(&count, &oldest); err != nil {
		return 0, time.Time{}, fmt.Errorf("counting %s usage for candidate %s: %w", action, candidateID, err)
	}
	return count, oldest, nil
}

func CountDistinctCandidates(ctx context.Context, pool *pgxpool.Pool, recruiterID, action string) (int, error) {
	const q = `SELECT count(DISTINCT candidate_id) FROM usage_events WHERE recruiter_id = $1 AND action = $2`

	var count int
	if err := pool.QueryRow(ctx, q, recruiterID, action).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting distinct %s candidates for recruiter %s: %w", action, recruiterID, err)
	}
	return count, nil
}

func CandidateHasUsage(ctx context.Context, pool *pgxpool.Pool, recruiterID, candidateID, action string) (bool, error) {
	const q = `SELECT true FROM usage_events WHERE recruiter_id = $1 AND candidate_id = $2 AND action = $3 LIMIT 1`

	var found bool
	err := pool.QueryRow(ctx, q, recruiterID, candidateID, action).Scan(&found)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s usage for candidate %s: %w", action, candidateID, err)
	}
	return found, nil
}
