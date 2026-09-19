package scoring

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/go-github/v66/github"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/ranking"
)

type Result struct {
	Reasoning     *llm.ResolvedJobReasoning
	TopRepos      []ranking.ScoredRepo
	Contributions []ghextractor.RawContribution

	Warning string

	ContributionsError string
}

type GithubEvidence struct {
	TopRepos           []ranking.ScoredRepo
	Contributions      []ghextractor.RawContribution
	ContributionsError string
}

func ExtractGithubEvidence(
	ctx context.Context,
	githubClient *github.Client,
	username string,
	jobStack []string,
) (*GithubEvidence, error) {
	allRepos, err := ghextractor.FetchOwnedRepos(ctx, githubClient, username)
	if err != nil {
		return nil, fmt.Errorf("fetching repos for %s: %w", username, err)
	}

	topRepos, err := ranking.BuildTopRepos(ctx, githubClient, username, allRepos, jobStack, 0)
	if err != nil {
		return nil, fmt.Errorf("ranking repos for %s: %w", username, err)
	}

	contributions, contribErr := ghextractor.FetchExternalContributions(ctx, githubClient, username)
	var contribErrMsg string
	if contribErr != nil {
		contribErrMsg = contribErr.Error()
		contributions = nil
	}

	return &GithubEvidence{
		TopRepos:           topRepos,
		Contributions:      contributions,
		ContributionsError: contribErrMsg,
	}, nil
}

func ScoreWithEvidence(
	ctx context.Context,
	mistralClient *llm.Client,
	username string,
	job llm.JobContext,
	evidence *GithubEvidence,
) (*Result, error) {
	if len(evidence.TopRepos) == 0 && len(evidence.Contributions) == 0 {
		return &Result{
			Reasoning: &llm.ResolvedJobReasoning{
				Score:      0,
				StackMatch: "weak",
			},
			TopRepos:           nil,
			Contributions:      evidence.Contributions,
			ContributionsError: evidence.ContributionsError,
			Warning:            "no owned repos or external contributions found — insufficient evidence to score",
		}, nil
	}

	userPrompt := llm.BuildUserPrompt(job, evidence.TopRepos, evidence.Contributions)

	const scoringTemperature = 0.0
	const maxParseRetries = 2

	var reasoning *llm.JobReasoning
	var lastParseErr error
	var rawResponse string

	for attempt := 0; attempt <= maxParseRetries; attempt++ {
		resp, err := mistralClient.CompleteJSON(ctx, llm.SystemPrompt(), userPrompt, scoringTemperature)
		if err != nil {
			return nil, fmt.Errorf("scoring %s via LLM: %w", username, err)
		}
		rawResponse = resp

		parsed, parseErr := llm.ParseJobReasoning(rawResponse)
		if parseErr == nil {
			reasoning = parsed
			break
		}

		lastParseErr = parseErr
		log.Printf("scoring %s: LLM returned malformed JSON (attempt %d/%d): %v", username, attempt+1, maxParseRetries+1, parseErr)
	}

	if reasoning == nil {
		return nil, fmt.Errorf("parsing LLM response for %s after %d attempts: %w (raw: %s)", username, maxParseRetries+1, lastParseErr, truncate(rawResponse, 500))
	}

	llm.EnforceReasonLimits(reasoning)

	evidenceWarrantedFlag := anyRepoFlaggedPadding(evidence.TopRepos) || anyRepoFlaggedJunkOrEnv(evidence.TopRepos)
	ok, warning := llm.VerifyTrustFlagHonored(reasoning, evidenceWarrantedFlag)
	if !ok {
		reasoning.HasTrustFlag = true
	}

	evidenceSources := buildRepoEvidenceSources(evidence.TopRepos)
	evidenceSources = append(evidenceSources, buildContributionEvidenceSources(evidence.Contributions)...)
	resolved, unresolvedEvidence := llm.ResolveEvidence(reasoning, evidenceSources)
	if len(unresolvedEvidence) > 0 {
		log.Printf("scoring %s: LLM cited evidence names that don't match any real repo/contribution: %v", username, unresolvedEvidence)
	}

	return &Result{
		Reasoning:          resolved,
		TopRepos:           evidence.TopRepos,
		Contributions:      evidence.Contributions,
		ContributionsError: evidence.ContributionsError,
		Warning:            warning,
	}, nil
}

func ScoreCandidate(
	ctx context.Context,
	githubClient *github.Client,
	mistralClient *llm.Client,
	username string,
	job llm.JobContext,
) (*Result, error) {
	evidence, err := ExtractGithubEvidence(ctx, githubClient, username, job.Stack)
	if err != nil {
		return nil, err
	}
	return ScoreWithEvidence(ctx, mistralClient, username, job, evidence)
}

func buildRepoEvidenceSources(repos []ranking.ScoredRepo) []llm.RepoEvidenceSource {
	sources := make([]llm.RepoEvidenceSource, 0, len(repos))
	for _, r := range repos {
		src := llm.RepoEvidenceSource{
			Name:    r.Repo.Name,
			RepoURL: r.Repo.RepoURL,
		}
		if r.Liveness != nil && r.Liveness.IsLive {
			src.LiveURL = r.Liveness.URL
		}
		sources = append(sources, src)
	}
	return sources
}

func buildContributionEvidenceSources(contributions []ghextractor.RawContribution) []llm.RepoEvidenceSource {
	sources := make([]llm.RepoEvidenceSource, 0, len(contributions))
	for _, c := range contributions {
		sources = append(sources, llm.RepoEvidenceSource{
			Name:    c.RepoOwner + "/" + c.RepoName,
			RepoURL: c.RepoURL,
		})
	}
	return sources
}

func anyRepoFlaggedPadding(repos []ranking.ScoredRepo) bool {
	for _, r := range repos {
		if r.Activity != nil && r.Activity.SuspiciousPadding {
			return true
		}
	}
	return false
}

func anyRepoFlaggedJunkOrEnv(repos []ranking.ScoredRepo) bool {
	for _, r := range repos {
		if r.Tree != nil && r.Tree.Junk.HasIssue() {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func BuildReport(candidateID, jobID string, jobStack []string, result *Result) *models.EvidenceReport {
	evidence := make([]models.RepoEvidenceSummary, 0, len(result.TopRepos))
	for _, r := range result.TopRepos {
		summary := models.RepoEvidenceSummary{
			Name:               r.Repo.Name,
			Description:        r.Repo.Description,
			RepoURL:            r.Repo.RepoURL,
			Languages:          languagePercentages(r.Languages),
			DetectedStack:      r.DetectedStack,
			DetectedStackError: r.DetectedStackError,
			Score:              r.Score,
		}
		if r.Activity != nil {
			summary.Commits90d = r.Activity.TotalCommits90d
			summary.ActiveWeeks90d = r.Activity.ActiveWeeks90d
			summary.SuspiciousPadding = r.Activity.SuspiciousPadding
		}
		if r.Tree != nil {
			summary.HasReadme = r.Tree.HasReadme
			summary.ReadmeTrunced = r.Tree.ReadmeTrunced
			summary.JunkDirs = r.Tree.Junk.JunkDirs
			summary.EnvFilesPushed = r.Tree.Junk.EnvFiles
		}
		if r.Liveness != nil {
			summary.IsLive = r.Liveness.IsLive
			if r.Liveness.IsLive {
				summary.LiveURL = r.Liveness.URL
			}
		}
		evidence = append(evidence, summary)
	}

	return &models.EvidenceReport{
		CandidateID:   candidateID,
		JobID:         jobID,
		GeneratedAt:   time.Now(),
		Evidence:      evidence,
		Contributions: convertContributions(result.Contributions),
		Reasoning:     convertReasoning(result.Reasoning, result.TopRepos, result.Contributions, jobStack),
		Warning:       result.Warning,
	}
}

func convertContributions(contributions []ghextractor.RawContribution) []models.ContributionSummary {
	out := make([]models.ContributionSummary, 0, len(contributions))
	for _, c := range contributions {
		out = append(out, models.ContributionSummary{
			RepoOwner:        c.RepoOwner,
			RepoName:         c.RepoName,
			RepoURL:          c.RepoURL,
			PRTitle:          c.PRTitle,
			PRUrl:            c.PRUrl,
			MergedAt:         c.MergedAt,
			MergedPRCount:    c.MergedPRCount,
			ContributorCount: c.ContributorCount,
			Stars:            c.Stars,
		})
	}
	return out
}

func convertReasoning(r *llm.ResolvedJobReasoning, topRepos []ranking.ScoredRepo, contributions []ghextractor.RawContribution, jobStack []string) models.ReasoningSummary {
	breakdown := ComputeFinalScore(topRepos, contributions, r.Score, jobStack)

	return models.ReasoningSummary{
		Score: breakdown.Final,
		Breakdown: models.ScoreBreakdownSummary{
			StackMatch:       breakdown.StackMatch,
			StackCoverage:    convertStackCoverage(breakdown.StackCoverage),
			EvidenceStrength: breakdown.EvidenceStrength,
			Contributions:    breakdown.Contributions,
			LLMJudgment:      breakdown.LLMJudgment,
		},
		StackMatch:      breakdown.StackMatchLabel,
		PositiveReasons: convertReasons(r.PositiveReasons),
		NegativeReasons: convertReasons(r.NegativeReasons),
		HasTrustFlag:    r.HasTrustFlag,
	}
}

func convertStackCoverage(items []StackCoverageItem) []models.StackCoverageSummary {
	out := make([]models.StackCoverageSummary, 0, len(items))
	for _, item := range items {
		out = append(out, models.StackCoverageSummary{
			Technology: item.Technology,
			Percentage: item.Percentage,
			RepoCount:  item.RepoCount,
			Repos:      item.Repos,
		})
	}
	return out
}

func convertReasons(reasons []llm.ResolvedReason) []models.ReasonSummary {
	out := make([]models.ReasonSummary, 0, len(reasons))
	for _, r := range reasons {
		evidence := make([]models.EvidenceLinkSummary, 0, len(r.Evidence))
		for _, e := range r.Evidence {
			evidence = append(evidence, models.EvidenceLinkSummary{
				Project: e.Project,
				RepoURL: e.RepoURL,
				LiveURL: e.LiveURL,
			})
		}
		out = append(out, models.ReasonSummary{Point: r.Point, Evidence: evidence})
	}
	return out
}

func languagePercentages(langs ghextractor.LanguageBreakdown) map[string]float64 {
	if len(langs) == 0 {
		return nil
	}
	var total int
	for _, bytes := range langs {
		total += bytes
	}
	if total == 0 {
		return nil
	}
	out := make(map[string]float64, len(langs))
	for lang, bytes := range langs {
		out[lang] = float64(bytes) / float64(total) * 100
	}
	return out
}
