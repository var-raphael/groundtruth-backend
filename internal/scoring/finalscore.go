package scoring

import (
	"math"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/ranking"
)

type StackCoverageItem struct {
	Technology string   `json:"technology"`
	Percentage float64  `json:"percentage"`
	RepoCount  int      `json:"repoCount"`
	Repos      []string `json:"repos"`
}

type FinalScoreBreakdown struct {
	StackMatch       float64             `json:"stackMatch"`
	StackMatchLabel  string              `json:"stackMatchLabel"`
	StackCoverage    []StackCoverageItem `json:"stackCoverage"`
	EvidenceStrength float64             `json:"evidenceStrength"`
	Contributions    float64             `json:"contributions"`
	LLMJudgment      float64             `json:"llmJudgment"`
	Final            float64             `json:"final"`
}

const (
	weightStackMatch       = 0.25
	weightEvidenceStrength = 0.25
	weightContributions    = 0.15
	weightLLMJudgment      = 0.35
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
		StackMatchLabel:  stackMatchLabel(stackMatch),
		StackCoverage:    computeStackCoverage(topRepos, jobStack),
		EvidenceStrength: round1(evidenceStrength),
		Contributions:    round1(contribScore),
		LLMJudgment:      round1(llmJudgment),
		Final:            round1(final),
	}
}

const minContributorsForRealContribution = 5
const contributionBaseScore = 6.0

const substantialContributionMinContributors = 20
const substantialContributionMinStars = 100
const substantialContributionCountForMaxScore = 3

func contributionsScore(contributions []ghextractor.RawContribution) float64 {
	maxContributors := 0
	qualifying := 0
	substantial := 0
	for _, c := range contributions {
		if c.ContributorCount < minContributorsForRealContribution {
			continue
		}
		qualifying++
		if c.ContributorCount > maxContributors {
			maxContributors = c.ContributorCount
		}
		if c.ContributorCount >= substantialContributionMinContributors && c.Stars >= substantialContributionMinStars {
			substantial++
		}
	}
	if qualifying == 0 {
		return 0
	}
	if substantial >= substantialContributionCountForMaxScore {
		return 10
	}

	bonus := math.Log10(float64(maxContributors)+1) - math.Log10(float64(minContributorsForRealContribution)+1)
	score := contributionBaseScore + bonus
	if score > 10 {
		score = 10
	}
	return score
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

func stackMatchLabel(stackMatch float64) string {
	if stackMatch >= 10.0 {
		return "strong"
	}
	if stackMatch > 0 {
		return "partial"
	}
	return "weak"
}

func languageCoveredAcross(lang string, repos []ranking.ScoredRepo) bool {
	for _, r := range repos {
		for _, detected := range r.DetectedStack {
			if ranking.StackNamesMatch(detected, lang) {
				return true
			}
		}
	}
	return false
}

func computeStackCoverage(repos []ranking.ScoredRepo, jobStack []string) []StackCoverageItem {
	if len(jobStack) == 0 || len(repos) == 0 {
		return nil
	}

	items := make([]StackCoverageItem, 0, len(jobStack))
	for _, tech := range jobStack {
		var matchingRepos []string
		for _, r := range repos {
			if repoUsesTechnology(r, tech) {
				matchingRepos = append(matchingRepos, r.Repo.Name)
			}
		}

		percentage := (float64(len(matchingRepos)) / float64(len(repos))) * 100.0

		items = append(items, StackCoverageItem{
			Technology: tech,
			Percentage: round1(percentage),
			RepoCount:  len(matchingRepos),
			Repos:      matchingRepos,
		})
	}

	return items
}

func repoUsesTechnology(r ranking.ScoredRepo, tech string) bool {
	for _, detected := range r.DetectedStack {
		if ranking.StackNamesMatch(detected, tech) {
			return true
		}
	}
	return false
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
