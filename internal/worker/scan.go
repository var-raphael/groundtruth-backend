package worker

import (
	"context"
	"log"
	"sync"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/scoring"
)

// extractedCandidate carries a candidate through from the GitHub phase to
// the LLM phase, so the LLM workers don't need to re-fetch anything.
type extractedCandidate struct {
	candidate models.Candidate
	job       models.Job
	evidence  *scoring.GithubEvidence
}

// Scan processes every queued candidate across all jobs in two phases:
//
//  1. GitHub extraction, fully concurrent — each candidate authenticates
//     with their own GitHub token, so there's no shared rate limit and no
//     concurrency cap is needed here.
//  2. LLM scoring, capped at exactly len(mistralClients) concurrent
//     workers — Mistral is a shared resource, so each worker is pinned to
//     its own API key/client to spread load without coordinating shared
//     rate-limit state between goroutines.
//
// mistralClients must have at least one entry; one LLM worker is started
// per client.
func Scan(ctx context.Context, pool *pgxpool.Pool, githubClient *github.Client, mistralClients []*llm.Client) error {
	jobCandidates, err := allQueuedCandidates(ctx, pool)
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
			runExtraction(ctx, pool, githubClient, jc, extracted)
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
				runScoring(ctx, pool, client, ec)
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

// allQueuedCandidates fetches every job's queued candidates. Candidates and
// jobs are queried per-job since ListCandidatesByJob is scoped to a single
// job; for the volumes this system expects, an N+1-style query here is not
// worth the complexity of a cross-job join.
func allQueuedCandidates(ctx context.Context, pool *pgxpool.Pool) ([]jobCandidate, error) {
	rows, err := pool.Query(ctx, `SELECT DISTINCT job_id FROM candidates WHERE status = $1`, models.StatusQueued)
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
		candidates, err := queries.ListCandidatesByJob(ctx, pool, jobID, models.StatusQueued)
		if err != nil {
			continue
		}
		for _, c := range candidates {
			result = append(result, jobCandidate{candidate: c, job: *job})
		}
	}
	return result, nil
}

func runExtraction(ctx context.Context, pool *pgxpool.Pool, githubClient *github.Client, jc jobCandidate, out chan<- extractedCandidate) {
	log.Printf("scan: [github] extracting %s (job %s)", jc.candidate.GithubUsername, jc.job.Title)

	if err := queries.UpdateCandidateStatus(ctx, pool, jc.candidate.ID, models.StatusScoring); err != nil {
		log.Printf("worker: marking candidate %s scoring: %v", jc.candidate.ID, err)
	}

	candidateGithubClient := githubClient
	if jc.candidate.GithubToken != "" {
		candidateGithubClient = ghextractor.NewClient(jc.candidate.GithubToken)
	}

	evidence, err := scoring.ExtractGithubEvidence(ctx, candidateGithubClient, jc.candidate.GithubUsername, jc.job.Stack)
	if err != nil {
		log.Printf("scan: [github] FAILED %s: %v", jc.candidate.GithubUsername, err)
		if updateErr := queries.UpdateCandidateStatus(ctx, pool, jc.candidate.ID, models.StatusFailed); updateErr != nil {
			log.Printf("worker: marking candidate %s failed: %v", jc.candidate.ID, updateErr)
		}
		return
	}

	log.Printf("scan: [github] done %s — %d repos, %d contributions", jc.candidate.GithubUsername, len(evidence.TopRepos), len(evidence.Contributions))
	out <- extractedCandidate{candidate: jc.candidate, job: jc.job, evidence: evidence}
}

func runScoring(ctx context.Context, pool *pgxpool.Pool, mistralClient *llm.Client, ec extractedCandidate) {
	jobCtx := llm.JobContext{
		Title:       ec.job.Title,
		Description: ec.job.Description,
		Stack:       ec.job.Stack,
	}

	result, err := scoring.ScoreWithEvidence(ctx, mistralClient, ec.candidate.GithubUsername, jobCtx, ec.evidence)
	if err != nil {
		log.Printf("scan: [llm] FAILED %s: %v", ec.candidate.GithubUsername, err)
		if updateErr := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusFailed); updateErr != nil {
			log.Printf("worker: marking candidate %s failed: %v", ec.candidate.ID, updateErr)
		}
		return
	}

	info := scoring.CandidateInfo{
		ID:              ec.candidate.ID,
		Name:            ec.candidate.FullName,
		GithubUsername:  ec.candidate.GithubUsername,
		Email:           ec.candidate.Email,
		Country:         ec.candidate.Country,
		YearsExperience: ec.candidate.YearsExperience,
		LinkedIn:        ec.candidate.LinkedIn,
		X:               ec.candidate.X,
		Portfolio:       ec.candidate.Portfolio,
	}

	report := scoring.BuildReport(ec.candidate.ID, ec.job.ID, info, ec.job.Stack, result)

	if err := queries.SaveReport(ctx, pool, report); err != nil {
		log.Printf("worker: saving report for candidate %s: %v", ec.candidate.ID, err)
		if updateErr := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusFailed); updateErr != nil {
			log.Printf("worker: marking candidate %s failed: %v", ec.candidate.ID, updateErr)
		}
		return
	}

	if err := queries.UpdateCandidateStatus(ctx, pool, ec.candidate.ID, models.StatusScored); err != nil {
		log.Printf("worker: marking candidate %s scored: %v", ec.candidate.ID, err)
		return
	}
	log.Printf("scan: [llm] done %s — score %d, stack match: %s", ec.candidate.GithubUsername, result.Reasoning.Score, result.Reasoning.StackMatch)
}
