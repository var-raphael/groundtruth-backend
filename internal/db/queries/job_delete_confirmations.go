package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateJobDeleteConfirmation(ctx context.Context, pool *pgxpool.Pool, jobID string) (string, error) {
	const q = `INSERT INTO job_delete_confirmations (job_id) VALUES ($1) RETURNING token`

	var token string
	if err := pool.QueryRow(ctx, q, jobID).Scan(&token); err != nil {
		return "", fmt.Errorf("creating delete confirmation for job %s: %w", jobID, err)
	}
	return token, nil
}

func MarkJobDeleteConfirmationDownloaded(ctx context.Context, pool *pgxpool.Pool, jobID string) error {
	const q = `UPDATE job_delete_confirmations SET downloaded = true WHERE job_id = $1 AND expires_at > now()`

	_, err := pool.Exec(ctx, q, jobID)
	if err != nil {
		return fmt.Errorf("marking delete confirmation downloaded for job %s: %w", jobID, err)
	}
	return nil
}

func ConsumeJobDeleteConfirmation(ctx context.Context, pool *pgxpool.Pool, jobID, token string) (bool, error) {
	const q = `
		DELETE FROM job_delete_confirmations
		WHERE token = $1 AND job_id = $2 AND downloaded = true AND expires_at > now()`

	tag, err := pool.Exec(ctx, q, token, jobID)
	if err != nil {
		return false, fmt.Errorf("consuming delete confirmation for job %s: %w", jobID, err)
	}
	return tag.RowsAffected() > 0, nil
}
