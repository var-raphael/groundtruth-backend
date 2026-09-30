package queries

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/timezone"
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

	job, err := GetJob(ctx, pool, jobID)
	if err != nil {
		return nil, fmt.Errorf("joining job for report %s: %w", candidateID, err)
	}
	var overlapHours int
	if job != nil {
		overlapHours = timezone.BestOverlapHours(candidate.Timezone, job.Timezones)
	}

	report := assembleReport(evidence, candidate.ToSummary())
	report.OverlapHours = overlapHours
	return report, nil
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

func AllReportsByJob(ctx context.Context, pool *pgxpool.Pool, jobID string) ([]models.CandidateReport, error) {
	const q = `SELECT candidate_id, report FROM candidate_reports WHERE job_id = $1 ORDER BY score DESC`

	rows, err := pool.Query(ctx, q, jobID)
	if err != nil {
		return nil, fmt.Errorf("listing all reports for job %s: %w", jobID, err)
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
			return nil, fmt.Errorf("scanning report row: %w", err)
		}
		var evidence models.EvidenceReport
		if err := json.Unmarshal(body, &evidence); err != nil {
			return nil, fmt.Errorf("unmarshaling report: %w", err)
		}
		all = append(all, pending{candidateID: candidateID, evidence: evidence})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	reports := make([]models.CandidateReport, 0, len(all))
	for _, p := range all {
		candidate, err := GetCandidate(ctx, pool, p.candidateID)
		if err != nil {
			return nil, fmt.Errorf("joining candidate identity for report %s: %w", p.candidateID, err)
		}
		if candidate == nil {
			continue
		}
		reports = append(reports, *assembleReport(p.evidence, candidate.ToSummary()))
	}
	return reports, nil
}

// ListReportsAndPendingByJob returns every candidate for a job, scored ones
// first (highest score first) with their full report, then unscored ones
// ordered oldest-applied first with Pending set and no report body. This is
// what listing/view endpoints should use instead of AllReportsByJob /
// ListReportsByJob so candidates mid-pipeline aren't just missing from the
// page. Pagination applies over the combined set, not the scored subset.
func ListReportsAndPendingByJob(ctx context.Context, pool *pgxpool.Pool, jobID string, limit, offset int) ([]models.CandidateReport, int, error) {
	var total int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM candidates WHERE job_id = $1`, jobID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting candidates for job %s: %w", jobID, err)
	}

	job, err := GetJob(ctx, pool, jobID)
	if err != nil {
		return nil, 0, fmt.Errorf("loading job %s for overlap calculation: %w", jobID, err)
	}
	var jobTimezones []string
	if job != nil {
		jobTimezones = job.Timezones
	}

	const q = `
		SELECT c.id, c.full_name, c.email, c.country, c.city, c.timezone, c.years_experience,
			c.github_id, c.github_username, c.linkedin, c.x, c.portfolio, c.status, c.status_updated_at, c.applied_at,
			cr.report
		FROM candidates c
		LEFT JOIN candidate_reports cr ON cr.candidate_id = c.id AND cr.job_id = c.job_id
		WHERE c.job_id = $1
		ORDER BY (cr.report IS NULL OR c.status != 'scored') ASC, cr.score DESC NULLS LAST, c.applied_at ASC
		LIMIT $2 OFFSET $3`

	rows, err := pool.Query(ctx, q, jobID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing reports and pending candidates for job %s: %w", jobID, err)
	}
	defer rows.Close()

	reports := make([]models.CandidateReport, 0, limit)
	for rows.Next() {
		var c models.Candidate
		var reportBody []byte
		if err := rows.Scan(
			&c.ID, &c.FullName, &c.Email, &c.Country, &c.City, &c.Timezone, &c.YearsExperience,
			&c.GithubID, &c.GithubUsername, &c.LinkedIn, &c.X, &c.Portfolio, &c.Status, &c.StatusUpdatedAt, &c.AppliedAt,
			&reportBody,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning candidate/report row: %w", err)
		}
		c.JobID = jobID

		overlapHours := timezone.BestOverlapHours(c.Timezone, jobTimezones)
		isPending := c.Status != models.StatusScored

		if reportBody == nil || isPending {
			reports = append(reports, models.CandidateReport{
				CandidateID:  c.ID,
				JobID:        jobID,
				Candidate:    c.ToSummary(),
				Pending:      true,
				OverlapHours: overlapHours,
			})
			continue
		}

		var evidence models.EvidenceReport
		if err := json.Unmarshal(reportBody, &evidence); err != nil {
			return nil, 0, fmt.Errorf("unmarshaling report for candidate %s: %w", c.ID, err)
		}
		report := assembleReport(evidence, c.ToSummary())
		report.OverlapHours = overlapHours
		reports = append(reports, *report)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

func assembleReport(evidence models.EvidenceReport, candidate models.CandidateSummary) *models.CandidateReport {
	return &models.CandidateReport{
		CandidateID:   evidence.CandidateID,
		JobID:         evidence.JobID,
		GeneratedAt:   evidence.GeneratedAt,
		LastScannedAt: evidence.GeneratedAt,
		Candidate:     candidate,
		Evidence:      evidence.Evidence,
		Contributions: evidence.Contributions,
		Reasoning:     evidence.Reasoning,
		Warning:       evidence.Warning,
	}
}
