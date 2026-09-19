package models

import "time"

type CandidateReport struct {
	CandidateID string    `json:"candidateId"`
	JobID       string    `json:"jobId"`
	GeneratedAt time.Time `json:"generatedAt"`

	Candidate CandidateSummary `json:"candidate"`

	Evidence []RepoEvidenceSummary `json:"evidence"`

	Contributions []ContributionSummary `json:"contributions,omitempty"`

	Reasoning ReasoningSummary `json:"reasoning"`

	Warning string `json:"warning,omitempty"`
}

type EvidenceReport struct {
	CandidateID string    `json:"candidateId"`
	JobID       string    `json:"jobId"`
	GeneratedAt time.Time `json:"generatedAt"`

	Evidence []RepoEvidenceSummary `json:"evidence"`

	Contributions []ContributionSummary `json:"contributions,omitempty"`

	Reasoning ReasoningSummary `json:"reasoning"`

	Warning string `json:"warning,omitempty"`
}

type CandidateSummary struct {
	CandidateID    string `json:"candidateId"`
	Name           string `json:"name"`
	GithubID       int64  `json:"githubId,omitempty"`
	GithubUsername string `json:"githubUsername"`
	Email          string `json:"email"`
	Country        string `json:"country"`
	City           string `json:"city,omitempty"`
	Timezone       string `json:"timezone,omitempty"`

	ClaimedExperienceYears int `json:"claimedExperienceYears"`

	LinkedIn  string `json:"linkedin,omitempty"`
	X         string `json:"x,omitempty"`
	Portfolio string `json:"portfolio,omitempty"`

	Status          string    `json:"status"`
	StatusUpdatedAt time.Time `json:"statusUpdatedAt"`
	AppliedAt       time.Time `json:"appliedAt"`
}

type RepoEvidenceSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	RepoURL     string `json:"repoUrl"`
	LiveURL     string `json:"liveUrl,omitempty"`
	IsLive      bool   `json:"isLive"`

	Languages map[string]float64 `json:"languages"`

	DetectedStack      []string `json:"detectedStack,omitempty"`
	DetectedStackError string   `json:"detectedStackError,omitempty"`

	Commits90d        int  `json:"commits90d"`
	ActiveWeeks90d    int  `json:"activeWeeks90d"`
	SuspiciousPadding bool `json:"suspiciousPadding"`

	JunkDirs       []string `json:"junkDirs,omitempty"`
	EnvFilesPushed []string `json:"envFilesPushed,omitempty"`

	HasReadme     bool `json:"hasReadme"`
	ReadmeTrunced bool `json:"readmeTruncated"`

	Score float64 `json:"score"`
}

type ReasoningSummary struct {
	Score      float64             `json:"score"`
	Breakdown  ScoreBreakdownSummary `json:"breakdown"`
	StackMatch string              `json:"stackMatch"`

	PositiveReasons []ReasonSummary `json:"positiveReasons"`
	NegativeReasons []ReasonSummary `json:"negativeReasons"`

	HasTrustFlag bool `json:"hasTrustFlag"`
}

type ScoreBreakdownSummary struct {
	StackMatch       float64                `json:"stackMatch"`
	StackCoverage    []StackCoverageSummary `json:"stackCoverage"`
	EvidenceStrength float64                `json:"evidenceStrength"`
	Contributions    float64                `json:"contributions"`
	LLMJudgment      float64                `json:"llmJudgment"`
}

type StackCoverageSummary struct {
	Technology string   `json:"technology"`
	Percentage float64  `json:"percentage"`
	RepoCount  int      `json:"repoCount"`
	Repos      []string `json:"repos"`
}

type ReasonSummary struct {
	Point    string                 `json:"point"`
	Evidence []EvidenceLinkSummary `json:"evidence"`
}

type EvidenceLinkSummary struct {
	Project string `json:"project"`
	RepoURL string `json:"repoUrl,omitempty"`
	LiveURL string `json:"liveUrl,omitempty"`
}

type ContributionSummary struct {
	RepoOwner        string `json:"repoOwner"`
	RepoName         string `json:"repoName"`
	RepoURL          string `json:"repoUrl"`
	PRTitle          string `json:"prTitle"`
	PRUrl            string `json:"prUrl"`
	MergedAt         string `json:"mergedAt"`
	MergedPRCount    int    `json:"mergedPrCount"`
	ContributorCount int    `json:"contributorCount"`
	Stars            int    `json:"stars"`
}
