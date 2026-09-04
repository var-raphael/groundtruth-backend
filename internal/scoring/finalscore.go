package scoring

import (
	"math"
	"strings"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/ranking"
)

type FinalScoreBreakdown struct {
	StackMatch       float64 `json:"stackMatch"`
	EvidenceStrength float64 `json:"evidenceStrength"`
	Contributions    float64 `json:"contributions"`
	LLMJudgment      float64 `json:"llmJudgment"`
	Final            float64 `json:"final"`
}

const (
	weightStackMatch       = 0.25
	weightEvidenceStrength = 0.25
	weightContributions    = 0.10
	weightLLMJudgment      = 0.40
)

func ComputeFinalScore(topRepos []ranking.ScoredRepo, contributions []ghextractor.RawContribution, llmScore int, jobStack []string) FinalScoreBreakdown {
	stackMatch := combinedStackMatch(topRepos, jobStack)
	evidenceStrength := normalizedAverageEvidenceStrength(topRepos)
	contribScore := contributionsScore(contributions)
	llmJudgment := float64(llmScore)

	final := stackMatch*weightStackMatch +
		evidenceStrength*weightEvidenceStrength +
		contribScore*weightContributions +
		llmJudgment*weightLLMJudgment

	return FinalScoreBreakdown{
		StackMatch:       round1(stackMatch),
		EvidenceStrength: round1(evidenceStrength),
		Contributions:    round1(contribScore),
		LLMJudgment:      round1(llmJudgment),
		Final:            round1(final),
	}
}

// contributionsScore log-scales each PR by the target repo's contributor count, sums, caps at 10 — no threshold gate, small repos still count, big ones count more.
func contributionsScore(contributions []ghextractor.RawContribution) float64 {
	if len(contributions) == 0 {
		return 0
	}
	var total float64
	for _, c := range contributions {
		total += math.Log10(float64(c.ContributorCount) + 1)
	}
	if total > 10 {
		total = 10
	}
	return total
}

func combinedStackMatch(repos []ranking.ScoredRepo, jobStack []string) float64 {
	if len(jobStack) == 0 || len(repos) == 0 {
		return 0
	}

	covered := 0
	for _, lang := range jobStack {
		if languageCoveredAcross(lang, repos) {
			covered++
		}
	}

	ratio := float64(covered) / float64(len(jobStack))
	return ratio * 10.0
}

const strongLanguagePresence = 0.15
const weakLanguagePresence = 0.03
const minReposForWeakPattern = 2

func languageCoveredAcross(lang string, repos []ranking.ScoredRepo) bool {
	reposWithWeakPresence := 0

	for _, r := range repos {
		if r.Languages == nil {
			continue
		}
		var totalBytes int
		for _, bytes := range r.Languages {
			totalBytes += bytes
		}
		if totalBytes == 0 {
			continue
		}
		for reportedLang, bytes := range r.Languages {
			if !strings.EqualFold(reportedLang, lang) {
				continue
			}
			proportion := float64(bytes) / float64(totalBytes)
			if proportion >= strongLanguagePresence {
				return true
			}
			if proportion >= weakLanguagePresence {
				reposWithWeakPresence++
			}
		}
	}

	return reposWithWeakPresence >= minReposForWeakPattern
}

func normalizedAverageEvidenceStrength(repos []ranking.ScoredRepo) float64 {
	if len(repos) == 0 {
		return 0
	}
	var total float64
	for _, r := range repos {
		total += r.NonStackScore()
	}
	avg := total / float64(len(repos))
	return (avg / ranking.MaxNonStackScore) * 10.0
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
