package queries

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
)

// SaveReport upserts a candidate's report for a job. Called once the scoring
// pipeline finishes (see internal/scoring/pipeline.go's BuildReport).
func SaveReport(ctx context.Context, pool *pgxpool.Pool, report *models.CandidateReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}

	const q = `
		INSERT INTO candidate_reports (candidate_id, job_id, score, stack_match, has_trust_flag, report, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (candidate_id, job_id) DO UPDATE SET
			score = EXCLUDED.score,
			stack_match = EXCLUDED.stack_match,
			has_trust_flag = EXCLUDED.has_trust_flag,
			report = EXCLUDED.report,
			generated_at = EXCLUDED.generated_at`

	_, err = pool.Exec(ctx, q,
		report.CandidateID, report.JobID,
		report.Reasoning.Score, report.Reasoning.StackMatch, report.Reasoning.HasTrustFlag,
		body, report.GeneratedAt,
	)
	if err != nil {
		return fmt.Errorf("saving report for candidate %s / job %s: %w", report.CandidateID, report.JobID, err)
	}
	return nil
}

// GetReport fetches a single candidate's report for a job. Returns nil, nil if not found.
func GetReport(ctx context.Context, pool *pgxpool.Pool, candidateID, jobID string) (*models.CandidateReport, error) {
	const q = `SELECT report FROM candidate_reports WHERE candidate_id = $1 AND job_id = $2`

	var body []byte
	err := pool.QueryRow(ctx, q, candidateID, jobID).Scan(&body)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fetching report for candidate %s / job %s: %w", candidateID, jobID, err)
	}

	var report models.CandidateReport
	if err := json.Unmarshal(body, &report); err != nil {
		return nil, fmt.Errorf("unmarshaling report: %w", err)
	}
	return &report, nil
}

// ListReportsByJob returns all candidate reports for a job, ranked by score descending.
// This is the primary query for the recruiter's ranked candidate list view.
func ListReportsByJob(ctx context.Context, pool *pgxpool.Pool, jobID string) ([]models.CandidateReport, error) {
	const q = `SELECT report FROM candidate_reports WHERE job_id = $1 ORDER BY score DESC`

	rows, err := pool.Query(ctx, q, jobID)
	if err != nil {
		return nil, fmt.Errorf("listing reports for job %s: %w", jobID, err)
	}
	defer rows.Close()

	var reports []models.CandidateReport
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, fmt.Errorf("scanning report row: %w", err)
		}
		var report models.CandidateReport
		if err := json.Unmarshal(body, &report); err != nil {
			return nil, fmt.Errorf("unmarshaling report: %w", err)
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}
