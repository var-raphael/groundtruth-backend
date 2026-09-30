package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type JobShareLink struct {
	Token     string
	JobID     string
	CreatedAt time.Time
	RevokedAt *time.Time
}

func CreateJobShareLink(ctx context.Context, pool *pgxpool.Pool, jobID string) (*JobShareLink, error) {
	const q = `
		INSERT INTO job_share_links (job_id)
		VALUES ($1)
		RETURNING token, job_id, created_at, revoked_at`

	link := &JobShareLink{}
	if err := pool.QueryRow(ctx, q, jobID).Scan(&link.Token, &link.JobID, &link.CreatedAt, &link.RevokedAt); err != nil {
		return nil, fmt.Errorf("creating share link for job %s: %w", jobID, err)
	}
	return link, nil
}

func GetActiveJobShareLinkByJob(ctx context.Context, pool *pgxpool.Pool, jobID string) (*JobShareLink, error) {
	const q = `
		SELECT token, job_id, created_at, revoked_at
		FROM job_share_links
		WHERE job_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1`

	link := &JobShareLink{}
	err := pool.QueryRow(ctx, q, jobID).Scan(&link.Token, &link.JobID, &link.CreatedAt, &link.RevokedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting active share link for job %s: %w", jobID, err)
	}
	return link, nil
}

func GetJobShareLinkByToken(ctx context.Context, pool *pgxpool.Pool, token string) (*JobShareLink, error) {
	const q = `
		SELECT token, job_id, created_at, revoked_at
		FROM job_share_links
		WHERE token = $1`

	link := &JobShareLink{}
	err := pool.QueryRow(ctx, q, token).Scan(&link.Token, &link.JobID, &link.CreatedAt, &link.RevokedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting share link by token: %w", err)
	}
	return link, nil
}

func RevokeJobShareLink(ctx context.Context, pool *pgxpool.Pool, jobID, token string) (bool, error) {
	const q = `
		UPDATE job_share_links
		SET revoked_at = now()
		WHERE token = $1 AND job_id = $2 AND revoked_at IS NULL`

	tag, err := pool.Exec(ctx, q, token, jobID)
	if err != nil {
		return false, fmt.Errorf("revoking share link for job %s: %w", jobID, err)
	}
	return tag.RowsAffected() > 0, nil
}
