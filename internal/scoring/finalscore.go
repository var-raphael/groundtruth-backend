package scoring

import (
	"math"
	"sort"

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

const contributionMinContributors = 20
const contributionMinStars = 100
const contributionMaxRanked = 5
const contributionQuantityWeight = 0.35
const contributionCredibilityWeight = 1 - contributionQuantityWeight

func contributionsScore(contributions []ghextractor.RawContribution) float64 {
	var qualifying []ghextractor.RawContribution
	for _, c := range contributions {
		if c.ContributorCount >= contributionMinContributors && c.Stars >= contributionMinStars {
			qualifying = append(qualifying, c)
		}
	}
	if len(qualifying) == 0 {
		return 0
	}

	sort.Slice(qualifying, func(i, j int) bool {
		return contributionCredibility(qualifying[i]) > contributionCredibility(qualifying[j])
	})
	best := qualifying
	if len(best) > contributionMaxRanked {
		best = best[:contributionMaxRanked]
	}

	quantity := float64(len(qualifying)) / float64(contributionMaxRanked)
	if quantity > 1 {
		quantity = 1
	}

	var totalCredibility float64
	for _, c := range best {
		totalCredibility += contributionCredibility(c)
	}
	avgCredibility := totalCredibility / float64(len(best))

	score := 10 * (contributionQuantityWeight*quantity + contributionCredibilityWeight*avgCredibility)
	if score > 10 {
		score = 10
	}
	return score
}

func contributionCredibility(c ghextractor.RawContribution) float64 {
	prTerm := math.Log10(float64(c.MergedPRCount)+1) / math.Log10(51) * 0.4
	contributorTerm := math.Log10(float64(c.ContributorCount)+1) / math.Log10(501) * 0.3
	starTerm := math.Log10(float64(c.Stars)+1) / math.Log10(100001) * 0.3
	return prTerm + contributorTerm + starTerm
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
