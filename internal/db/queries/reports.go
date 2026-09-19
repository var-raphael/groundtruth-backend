package queries

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
)

func SaveReport(ctx context.Context, pool *pgxpool.Pool, report *models.EvidenceReport) error {
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

	var evidence models.EvidenceReport
	if err := json.Unmarshal(body, &evidence); err != nil {
		return nil, fmt.Errorf("unmarshaling report: %w", err)
	}

	candidate, err := GetCandidate(ctx, pool, candidateID)
	if err != nil {
		return nil, fmt.Errorf("joining candidate identity for report %s: %w", candidateID, err)
	}
	if candidate == nil {
		return nil, fmt.Errorf("candidate %s referenced by report no longer exists", candidateID)
	}

	return assembleReport(evidence, candidate.ToSummary()), nil
}

func ListReportsByJob(ctx context.Context, pool *pgxpool.Pool, jobID string, limit, offset int) ([]models.CandidateReport, int, error) {
	var total int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM candidate_reports WHERE job_id = $1`, jobID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting reports for job %s: %w", jobID, err)
	}

	const q = `SELECT candidate_id, report FROM candidate_reports WHERE job_id = $1 ORDER BY score DESC LIMIT $2 OFFSET $3`

	rows, err := pool.Query(ctx, q, jobID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing reports for job %s: %w", jobID, err)
	}
	defer rows.Close()

	type pending struct {
		candidateID string
		evidence    models.EvidenceReport
	}
	var all []pending
	for rows.Next() {
		var candidateID string
		var body []byte
		if err := rows.Scan(&candidateID, &body); err != nil {
			return nil, 0, fmt.Errorf("scanning report row: %w", err)
		}
		var evidence models.EvidenceReport
		if err := json.Unmarshal(body, &evidence); err != nil {
			return nil, 0, fmt.Errorf("unmarshaling report: %w", err)
		}
		all = append(all, pending{candidateID: candidateID, evidence: evidence})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	reports := make([]models.CandidateReport, 0, len(all))
	for _, p := range all {
		candidate, err := GetCandidate(ctx, pool, p.candidateID)
		if err != nil {
			return nil, 0, fmt.Errorf("joining candidate identity for report %s: %w", p.candidateID, err)
		}
		if candidate == nil {
			continue
		}
		reports = append(reports, *assembleReport(p.evidence, candidate.ToSummary()))
	}
	return reports, total, nil
}

func assembleReport(evidence models.EvidenceReport, candidate models.CandidateSummary) *models.CandidateReport {
	return &models.CandidateReport{
		CandidateID:   evidence.CandidateID,
		JobID:         evidence.JobID,
		GeneratedAt:   evidence.GeneratedAt,
		Candidate:     candidate,
		Evidence:      evidence.Evidence,
		Contributions: evidence.Contributions,
		Reasoning:     evidence.Reasoning,
		Warning:       evidence.Warning,
	}
}
