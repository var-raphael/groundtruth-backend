package worker

import (
	"context"
	"log"
	"sync"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/ranking"
	"github.com/var-raphael/groundtruth/internal/scoring"
)

type extractedCandidate struct {
	candidate models.Candidate
	job       models.Job
	evidence  *scoring.GithubEvidence
}

type ScanOptions struct {
	Force         bool
	CandidateID   string
	ReuseEvidence bool
}

func Scan(ctx context.Context, pool *pgxpool.Pool, githubClient *github.Client, mistralClients []*llm.Client, opts ScanOptions) error {
	jobCandidates, err := eligibleCandidates(ctx, pool, opts)
	if err != nil {
		return err
	}
	if len(jobCandidates) == 0 {
		log.Printf("scan: no queued candidates found")
		return nil
	}
	log.Printf("scan: starting — %d queued candidate(s), %d LLM worker(s)", len(jobCandidates), len(mistralClients))

	extracted := make(chan extractedCandidate, len(jobCandidates))
	var extractWG sync.WaitGroup

	for _, jc := range jobCandidates {
		extractWG.Add(1)
		go func(jc jobCandidate) {
			defer extractWG.Done()
			runExtractionOrReuse(ctx, pool, githubClient, jc, opts.Force, opts.ReuseEvidence, extracted)
		}(jc)
	}

	go func() {
		extractWG.Wait()
		close(extracted)
		log.Printf("scan: github extraction phase complete")
	}()

	var scoreWG sync.WaitGroup
	for i, client := range mistralClients {
		scoreWG.Add(1)
		go func(workerNum int, client *llm.Client) {
			defer scoreWG.Done()
			for ec := range extracted {
				log.Printf("scan: [llm worker %d] scoring %s (job %s)", workerNum, ec.candidate.GithubUsername, ec.job.Title)
				runScoring(ctx, pool, githubClient, client, ec)
			}
		}(i+1, client)
	}
	scoreWG.Wait()

	log.Printf("scan: complete")
	return nil
}

type jobCandidate struct {
	candidate models.Candidate
	job       models.Job
}

func eligibleCandidates(ctx context.Context, pool *pgxpool.Pool, opts ScanOptions) ([]jobCandidate, error) {
	if opts.CandidateID != "" {
		c, err := queries.GetCandidate(ctx, pool, opts.CandidateID)
		if err != nil || c == nil {
			return nil, err
		}
		job, err := queries.GetJob(ctx, pool, c.JobID)
		if err != nil || job == nil {
			return nil, err
		}
		return []jobCandidate{{candidate: *c, job: *job}}, nil
	}

	statuses := []models.CandidateStatus{models.StatusQueued, models.StatusExtracted}

	rows, err := pool.Query(ctx, `SELECT DISTINCT job_id FROM candidates WHERE status = ANY($1)`, statuses)
	if err != nil {
		return nil, err
	}
	var jobIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		jobIDs = append(jobIDs, id)
	}
	rows.Close()

	var result []jobCandidate
	for _, jobID := range jobIDs {
		job, err := queries.GetJob(ctx, pool, jobID)
		if err != nil || job == nil {
			continue
		}
		for _, status := range statuses {
			candidates, err := queries.ListCandidatesByJob(ctx, pool, jobID, status)
			if err != nil {
				continue
			}
			for _, c := range candidates {
				result = append(result, jobCandidate{candidate: c, job: *job})
			}
		}
	}
	return result, nil
}

func runExtractionOrReuse(ctx context.Context, pool *pgxpool.Pool, githubClient *github.Client, jc jobCandidate, force bool, reuseEvidence bool, out chan<- extractedCandidate) {
	attemptReuse := !force && (reuseEvidence ||
		(jc.candidate.Status != models.StatusQueued &&
			jc.candidate.Status != models.StatusExtracting &&
			jc.candidate.Status != models.StatusScoring))

	if attemptReuse {
		evidence, err := queries.GetCandidateEvidence(ctx, pool, jc.candidate.ID)
		if err != nil {
			log.Printf("scan: [github] failed loading saved evidence for %s, falling back to fresh extraction: %v", jc.candidate.GithubUsername, err)
		} else if evidence != nil {
			log.Printf("scan: [github] reusing saved evidence for %s — %d repos, %d contributions", jc.candidate.GithubUsername, len(evidence.TopRepos), len(evidence.Contributions))
			out <- extractedCandidate{candidate: jc.candidate, job: jc.job, evidence: evidence}
			return
		}
	}

	runExtraction(ctx, pool, githubClient, jc, out)
}

func runExtraction(ctx context.Context, pool *pgxpool.Pool, githubClient *github.Client, jc jobCandidate, out chan<- extractedCandidate) {
	log.Printf("scan: [github] extracting %s (job %s)", jc.candidate.GithubUsername, jc.job.Title)

	if err := queries.UpdateCandidateStatus(ctx, pool, jc.candidate.ID, models.StatusExtracting); err != nil {
		log.Printf("worker: marking candidate %s extracting: %v", jc.candidate.ID, err)
	}

	candidateGithubClient := pickGithubClient(ctx, githubClient, &jc.candidate)

	evidence, err := scoring.ExtractGithubEvidence(ctx, candidateGithubClient, jc.candidate.GithubUsername, jc.job.Stack)
	if err != nil {
		log.Printf("scan: [github] FAILED %s: %v", jc.candidate.GithubUsername, err)
		if updateErr := queries.UpdateCandidateStatus(ctx, pool, jc.candidate.ID, models.StatusFailed); updateErr != nil {
			log.Printf("worker: marking candidate %s failed: %v", jc.candidate.ID, updateErr)
		}
		return
	}

	if err := queries.SaveCandidateEvidence(ctx, pool, jc.candidate.ID, evidence); err != nil {
		log.Printf("scan: [github] failed saving evidence snapshot for %s (continuing anyway): %v", jc.candidate.GithubUsername, err)
	} else if err := queries.UpdateCandidateStatus(ctx, pool, jc.candidate.ID, models.StatusExtracted); err != nil {
		log.Printf("worker: marking candidate %s extracted: %v", jc.candidate.ID, err)
	}

	log.Printf("scan: [github] done %s — %d repos, %d contributions", jc.candidate.GithubUsername, len(evidence.TopRepos), len(evidence.Contributions))
	out <- extractedCandidate{candidate: jc.candidate, job: jc.job, evidence: evidence}
}

func runScoring(ctx context.Context, pool *pgxpool.Pool, githubClient *github.Client, mistralClient *llm.Client, ec extractedCandidate) {
	if err := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusScoring); err != nil {
		log.Printf("worker: marking candidate %s scoring: %v", ec.candidate.ID, err)
	}

	jobCtx := llm.JobContext{
		Title:       ec.job.Title,
		Description: ec.job.Description,
		Stack:       ec.job.Stack,
	}

	candidateGithubClient := pickGithubClient(ctx, githubClient, &ec.candidate)

	allRepos := ec.evidence.TopRepos
	shortlist := ranking.ShortlistForDetection(allRepos, ec.job.Stack)
	shortlist = scoring.ApplyDetectedStack(ctx, candidateGithubClient, mistralClient, ec.candidate.GithubUsername, shortlist, ec.job.Stack)
	verified := ranking.SelectVerifiedTopRepos(shortlist, ec.job.Stack, 0)

	ec.evidence.StackUnmatched = false
	if len(verified) == 0 && len(allRepos) > 0 {
		verified = ranking.FallbackRepos(shortlist, allRepos)
		ec.evidence.StackUnmatched = len(verified) > 0
	}
	ec.evidence.TopRepos = verified

	result, err := scoring.ScoreWithEvidence(ctx, mistralClient, ec.candidate.GithubUsername, jobCtx, ec.evidence)
	if err != nil {
		log.Printf("scan: [llm] FAILED %s: %v (evidence snapshot preserved, will retry scoring only)", ec.candidate.GithubUsername, err)
		if updateErr := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusExtracted); updateErr != nil {
			log.Printf("worker: marking candidate %s extracted: %v", ec.candidate.ID, updateErr)
		}
		return
	}

	report := scoring.BuildReport(ec.candidate.ID, ec.job.ID, ec.job.Stack, result)

	if err := queries.SaveReport(ctx, pool, report); err != nil {
		log.Printf("worker: saving report for candidate %s: %v (evidence snapshot preserved, will retry scoring only)", ec.candidate.ID, err)
		if updateErr := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusExtracted); updateErr != nil {
			log.Printf("worker: marking candidate %s extracted: %v", ec.candidate.ID, updateErr)
		}
		return
	}

	if err := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusScored); err != nil {
		log.Printf("worker: marking candidate %s scored: %v", ec.candidate.ID, err)
		return
	}
	log.Printf("scan: [llm] done %s — score %.1f, stack match: %s", ec.candidate.GithubUsername, report.Reasoning.Score, report.Reasoning.StackMatch)
}
