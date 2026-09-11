package scoring

import (
	"context"
	"fmt"
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

// GithubEvidence holds everything gathered from GitHub for a candidate,
// independent of any job — the expensive, per-candidate-token part of
// scoring that has no shared rate limit and can run fully concurrently.
type GithubEvidence struct {
	TopRepos           []ranking.ScoredRepo
	Contributions      []ghextractor.RawContribution
	ContributionsError string
}

// ExtractGithubEvidence performs the GitHub-side extraction and ranking for
// a candidate against a job's stack. Each candidate authenticates with
// their own GitHub token, so this has no shared rate limit and callers may
// run many of these concurrently.
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

// ScoreWithEvidence runs the LLM scoring phase against already-extracted
// GitHub evidence. Mistral is a shared resource, so callers should bound
// concurrency for this phase (e.g. via a small worker pool), unlike
// ExtractGithubEvidence which has no such constraint.
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

	rawResponse, err := mistralClient.CompleteJSON(ctx, llm.SystemPrompt(), userPrompt, scoringTemperature)
	if err != nil {
		return nil, fmt.Errorf("scoring %s via LLM: %w", username, err)
	}

	reasoning, err := llm.ParseJobReasoning(rawResponse)
	if err != nil {
		return nil, fmt.Errorf("parsing LLM response for %s: %w (raw: %s)", username, err, truncate(rawResponse, 500))
	}

	llm.EnforceReasonLimits(reasoning)

	evidenceHadPadding := anyRepoFlaggedPadding(evidence.TopRepos)
	ok, warning := llm.VerifyTrustFlagHonored(reasoning, evidenceHadPadding)
	if !ok {
		reasoning.HasTrustFlag = true
	}

	evidenceSources := buildRepoEvidenceSources(evidence.TopRepos)
	evidenceSources = append(evidenceSources, buildContributionEvidenceSources(evidence.Contributions)...)
	resolved := llm.ResolveEvidence(reasoning, evidenceSources)

	return &Result{
		Reasoning:          resolved,
		TopRepos:           evidence.TopRepos,
		Contributions:      evidence.Contributions,
		ContributionsError: evidence.ContributionsError,
		Warning:            warning,
	}, nil
}

// ScoreCandidate runs both phases sequentially for a single candidate.
// Kept for callers (like cmd/testrun) that don't need the two phases split
// across separate concurrency pools — the worker uses ExtractGithubEvidence
// and ScoreWithEvidence directly instead.
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

type CandidateInfo struct {
	ID              string
	Name            string
	GithubUsername  string
	Email           string
	Country         string
	YearsExperience int
	LinkedIn        string
	X               string
	Portfolio       string
}

func BuildReport(candidateID, jobID string, info CandidateInfo, jobStack []string, result *Result) *models.CandidateReport {
	evidence := make([]models.RepoEvidenceSummary, 0, len(result.TopRepos))
	for _, r := range result.TopRepos {
		summary := models.RepoEvidenceSummary{
			Name:        r.Repo.Name,
			Description: r.Repo.Description,
			RepoURL:     r.Repo.RepoURL,
			Languages:   languagePercentages(r.Languages),
			Score:       r.Score,
		}
		if r.Activity != nil {
			summary.Commits90d = r.Activity.TotalCommits90d
			summary.ActiveWeeks90d = r.Activity.ActiveWeeks90d
			summary.SuspiciousPadding = r.Activity.SuspiciousPadding
		}
		if r.Tree != nil {
			summary.HasReadme = r.Tree.HasReadme
			summary.ReadmeTrunced = r.Tree.ReadmeTrunced
		}
		if r.Liveness != nil {
			summary.IsLive = r.Liveness.IsLive
			if r.Liveness.IsLive {
				summary.LiveURL = r.Liveness.URL
			}
		}
		evidence = append(evidence, summary)
	}

	return &models.CandidateReport{
		CandidateID: candidateID,
		JobID:       jobID,
		GeneratedAt: time.Now(),
		Candidate: models.CandidateSummary{
			Name:            info.Name,
			GithubUsername:  info.GithubUsername,
			Email:           info.Email,
			Country:         info.Country,
			YearsExperience: info.YearsExperience,
			LinkedIn:        info.LinkedIn,
			X:               info.X,
			Portfolio:       info.Portfolio,
		},
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
			EvidenceStrength: breakdown.EvidenceStrength,
			Contributions:    breakdown.Contributions,
			LLMJudgment:      breakdown.LLMJudgment,
		},
		StackMatch:      r.StackMatch,
		PositiveReasons: convertReasons(r.PositiveReasons),
		NegativeReasons: convertReasons(r.NegativeReasons),
		HasTrustFlag:    r.HasTrustFlag,
	}
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
