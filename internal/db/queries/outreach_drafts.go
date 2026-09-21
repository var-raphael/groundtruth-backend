package queries

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
)

func GetOutreachDraft(ctx context.Context, pool *pgxpool.Pool, candidateID string) (*models.OutreachDraft, error) {
	const q = `
		SELECT d.candidate_id, d.job_id, d.subject, d.body, d.edited, d.generated_at, d.updated_at,
			coalesce(r.generated_at > d.generated_at, false)
		FROM outreach_drafts d
		LEFT JOIN candidate_reports r ON r.candidate_id = d.candidate_id AND r.job_id = d.job_id
		WHERE d.candidate_id = $1`

	var d models.OutreachDraft
	err := pool.QueryRow(ctx, q, candidateID).Scan(
		&d.CandidateID, &d.JobID, &d.Subject, &d.Body, &d.Edited, &d.GeneratedAt, &d.UpdatedAt, &d.Stale,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching outreach draft for candidate %s: %w", candidateID, err)
	}
	return &d, nil
}

func SaveGeneratedOutreachDraft(ctx context.Context, pool *pgxpool.Pool, candidateID, jobID, subject, body string) error {
	const q = `
		INSERT INTO outreach_drafts (candidate_id, job_id, subject, body, edited, generated_at, updated_at)
		VALUES ($1, $2, $3, $4, false, now(), now())
		ON CONFLICT (candidate_id) DO UPDATE SET
			job_id = EXCLUDED.job_id,
			subject = EXCLUDED.subject,
			body = EXCLUDED.body,
			edited = false,
			generated_at = now(),
			updated_at = now()`

	if _, err := pool.Exec(ctx, q, candidateID, jobID, subject, body); err != nil {
		return fmt.Errorf("saving generated outreach draft for candidate %s: %w", candidateID, err)
	}
	return nil
}

func SaveEditedOutreachDraft(ctx context.Context, pool *pgxpool.Pool, candidateID, subject, body string) (bool, error) {
	const q = `
		UPDATE outreach_drafts
		SET subject = $2, body = $3, edited = true, updated_at = now()
		WHERE candidate_id = $1`

	tag, err := pool.Exec(ctx, q, candidateID, subject, body)
	if err != nil {
		return false, fmt.Errorf("saving edited outreach draft for candidate %s: %w", candidateID, err)
	}
	return tag.RowsAffected() > 0, nil
}
