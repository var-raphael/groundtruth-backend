package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CountDistinctShareLinkOutreachCandidates(ctx context.Context, pool *pgxpool.Pool, token string) (int, error) {
	const q = `SELECT count(DISTINCT candidate_id) FROM share_link_outreach_events WHERE share_token = $1`

	var count int
	if err := pool.QueryRow(ctx, q, token).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting distinct outreach candidates for share link %s: %w", token, err)
	}
	return count, nil
}

func ShareLinkHasOutreachForCandidate(ctx context.Context, pool *pgxpool.Pool, token, candidateID string) (bool, error) {
	const q = `SELECT true FROM share_link_outreach_events WHERE share_token = $1 AND candidate_id = $2 LIMIT 1`

	var found bool
	err := pool.QueryRow(ctx, q, token, candidateID).Scan(&found)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking outreach usage for share link %s: %w", token, err)
	}
	return found, nil
}

func RecordShareLinkOutreach(ctx context.Context, pool *pgxpool.Pool, token, candidateID string) error {
	const q = `INSERT INTO share_link_outreach_events (share_token, candidate_id) VALUES ($1, $2)`

	if _, err := pool.Exec(ctx, q, token, candidateID); err != nil {
		return fmt.Errorf("recording outreach usage for share link %s: %w", token, err)
	}
	return nil
}
