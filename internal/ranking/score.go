package ranking

import (
	"math"
	"strings"
	"time"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
)

type ScoredRepo struct {
	Repo      ghextractor.RawRepo
	Activity  *ghextractor.ActivitySummary
	Tree      *ghextractor.TreeSummary
	Languages ghextractor.LanguageBreakdown
	Liveness  *ghextractor.LivenessCheck
	Commits   []ghextractor.CommitTiming

	Score           float64
	StackMatchScore float64

	ActivityError  string `json:"ActivityError,omitempty"`
	TreeError      string `json:"TreeError,omitempty"`
	LanguagesError string `json:"LanguagesError,omitempty"`
	CommitsError   string `json:"CommitsError,omitempty"`
}

const MaxPossibleScore = 100.0

const MaxStackMatchScore = 40.0

const MaxNonStackScore = 60.0

const MaxRecencyScore = 25.0
const MaxActivityScore = 20.0
const MaxLivenessScore = 10.0
const MaxTreeQualityScore = 5.0

func (s ScoredRepo) NonStackScore() float64 {
	raw := s.Score - s.StackMatchScore
	ceiling := s.nonStackCeiling()
	if ceiling <= 0 {
		return 0
	}
	return (raw / ceiling) * MaxNonStackScore
}

func (s ScoredRepo) nonStackCeiling() float64 {
	if strings.TrimSpace(s.Repo.HomepageURL) == "" {
		return MaxNonStackScore - MaxLivenessScore
	}
	return MaxNonStackScore
}

func ScoreRepo(repo ghextractor.RawRepo, activity *ghextractor.ActivitySummary, tree *ghextractor.TreeSummary, languages ghextractor.LanguageBreakdown, jobStack []string) ScoredRepo {
	stackScore := stackMatchScore(languages, jobStack)

	score := stackScore
	score += recencyScore(repo.PushedAt)
	score += activityScore(activity)
	score += livenessScore(repo.HomepageURL, nil)
	score += treeQualityScore(tree)

	return ScoredRepo{
		Repo:            repo,
		Activity:        activity,
		Tree:            tree,
		Languages:       languages,
		Score:           score,
		StackMatchScore: stackScore,
	}
}

func stackMatchScore(languages ghextractor.LanguageBreakdown, jobStack []string) float64 {
	if len(jobStack) == 0 {
		return 0
	}
	if len(languages) == 0 {
		return 0
	}

	var totalBytes int
	for _, bytes := range languages {
		totalBytes += bytes
	}
	if totalBytes == 0 {
		return 0
	}

	const presenceFloor = 0.03

	matched := 0
	for _, lang := range jobStack {
		for reportedLang, bytes := range languages {
			if !strings.EqualFold(reportedLang, lang) {
				continue
			}
			proportion := float64(bytes) / float64(totalBytes)
			if proportion >= presenceFloor {
				matched++
			}
			break
		}
	}

	ratio := float64(matched) / float64(len(jobStack))
	const stackMatchWeight = 40.0
	return ratio * stackMatchWeight
}

func recencyScore(pushedAt string) float64 {
	t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", pushedAt)
	if err != nil {
		return 0
	}

	daysAgo := time.Since(t).Hours() / 24
	const recencyWeight = 25.0
	const halfLifeDays = 120.0

	decay := halfLifeDecay(daysAgo, halfLifeDays)
	return decay * recencyWeight
}

func halfLifeDecay(elapsed, halfLife float64) float64 {
	if elapsed <= 0 {
		return 1.0
	}
	return math.Pow(0.5, elapsed/halfLife)
}

func activityScore(activity *ghextractor.ActivitySummary) float64 {
	if activity == nil {
		return 0
	}

	const activityWeight = 20.0
	const commitCap = 100.0

	commits := float64(activity.TotalCommits90d)
	if commits > commitCap {
		commits = commitCap
	}
	volumeScore := (commits / commitCap) * (activityWeight * 0.7)

	weeksScore := (float64(activity.ActiveWeeks90d) / 13.0) * (activityWeight * 0.3)

	total := volumeScore + weeksScore

	return total
}

func livenessScore(homepageURL string, liveness *ghextractor.LivenessCheck) float64 {
	const livenessWeight = 10.0
	const unverifiedClaimCredit = 3.0

	if strings.TrimSpace(homepageURL) == "" {
		return 0
	}
	if liveness == nil {
		return unverifiedClaimCredit
	}
	if liveness.IsLive {
		return livenessWeight
	}
	return 0
}

func treeQualityScore(tree *ghextractor.TreeSummary) float64 {
	if tree == nil {
		return 0
	}
	const maxWeight = 5.0
	const fileCountCap = 30.0

	files := float64(len(tree.Paths))
	if files > fileCountCap {
		files = fileCountCap
	}
	score := (files / fileCountCap) * maxWeight

	if tree.HasReadme {
		score += 0
	}

	return score
}
