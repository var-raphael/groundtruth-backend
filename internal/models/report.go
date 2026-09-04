package models

import "time"

// CandidateReport is the complete, final output for one candidate scored
// against one job — this is what gets persisted and what the frontend
// renders. It deliberately keeps hard data and LLM-derived data as
// separate top-level sections rather than merging them, matching the
// FAQ's own stated principle: "facts and opinions live in two different
// sections... you can always tell which is which."
//
// Location/timezone matching against the job is deliberately NOT computed
// or stored here — Candidate.Country and Job.LocationCountries are both
// already real, live fields; the frontend filters/sorts on them directly
// rather than this report freezing a derived match flag that could drift
// out of sync with the job's actual requirements over time.
type CandidateReport struct {
	CandidateID string    `json:"candidateId"`
	JobID       string    `json:"jobId"`
	GeneratedAt time.Time `json:"generatedAt"`

	// --- hard candidate data: self-reported + GitHub-verified identity ---
	Candidate CandidateSummary `json:"candidate"`

	// --- hard evidence data: real, verified repo/activity data backing every claim ---
	Evidence []RepoEvidenceSummary `json:"evidence"`

	// --- hard evidence data: real, verified external open-source contributions ---
	Contributions []ContributionSummary `json:"contributions,omitempty"`

	// --- LLM-derived: score, stack match, reasons — clearly separate from the above ---
	Reasoning ReasoningSummary `json:"reasoning"`

	// Warning surfaces pipeline-level concerns (e.g. trust-flag mismatch
	// override, insufficient evidence) — shown to the recruiter as a
	// small caveat, not hidden.
	Warning string `json:"warning,omitempty"`
}

// CandidateSummary is the hard, factual identity data — nothing here is
// LLM-derived or scored, it's either self-reported at apply time or
// pulled directly from a verified source (GitHub OAuth).
type CandidateSummary struct {
	Name            string `json:"name"`
	GithubUsername  string `json:"githubUsername"`
	Email           string `json:"email"`
	Country         string `json:"country"` // self-reported at apply time; frontend filters against Job.LocationCountries directly, no derived match flag stored here
	YearsExperience int    `json:"yearsExperience"`

	LinkedIn  string `json:"linkedin,omitempty"`
	X         string `json:"x,omitempty"`
	Portfolio string `json:"portfolio,omitempty"`
}

// RepoEvidenceSummary is the hard, verified data for one of the
// candidate's top-K repos — everything a recruiter could independently
// check themselves. This is what a Reason's EvidenceLink points back
// into, and what the frontend's expandable repo card renders.
type RepoEvidenceSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	RepoURL     string `json:"repoUrl"`
	LiveURL     string `json:"liveUrl,omitempty"` // only set if actually verified live
	IsLive      bool   `json:"isLive"`

	Languages map[string]float64 `json:"languages"` // language -> percentage of repo bytes, real proportions not GitHub's single-guess field

	Commits90d        int  `json:"commits90d"`
	ActiveWeeks90d    int  `json:"activeWeeks90d"`
	SuspiciousPadding bool `json:"suspiciousPadding"`

	HasReadme     bool `json:"hasReadme"`
	ReadmeTrunced bool `json:"readmeTruncated"`

	Score float64 `json:"score"` // mechanical ranking score — shown for transparency, not the same as the LLM's 0-10 fit score
}

// ReasoningSummary is the LLM-derived judgment PLUS the final weighted
// score breakdown — kept as its own clean section so it's never visually
// or structurally confused with the hard data above it. Score is no
// longer the LLM's number in isolation; it's the transcript-style
// weighted total (see scoring.ComputeFinalScore) — Breakdown shows exactly
// how that total was reached.
type ReasoningSummary struct {
	Score      float64             `json:"score"`      // 0-10, the FINAL weighted score, not the LLM's number alone
	Breakdown  ScoreBreakdownSummary `json:"breakdown"` // transcript-style: how Score was actually reached
	StackMatch string              `json:"stackMatch"` // "strong" | "partial" | "weak" — the LLM's own categorical read, kept separate from the numeric breakdown

	PositiveReasons []ReasonSummary `json:"positiveReasons"` // max 10, ranked
	NegativeReasons []ReasonSummary `json:"negativeReasons"` // max 5, ranked; trust-flag reason always included if present

	HasTrustFlag bool `json:"hasTrustFlag"`
}

// ScoreBreakdownSummary mirrors scoring.FinalScoreBreakdown for the
// persisted/rendered report — kept as a separate type in models so this
// package doesn't need to import scoring directly.
type ScoreBreakdownSummary struct {
	StackMatch       float64 `json:"stackMatch"`
	EvidenceStrength float64 `json:"evidenceStrength"`
	Contributions    float64 `json:"contributions"`
	LLMJudgment      float64 `json:"llmJudgment"`
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

// ContributionSummary is one verified merged PR on a repo the candidate
// doesn't own — real, independently-reviewed external open-source work.
// Contributor count and stars are shown as-is so a recruiter can judge
// significance themselves, matching the same "receipts, not opinions"
// principle as everything else in this report.
type ContributionSummary struct {
	RepoOwner        string `json:"repoOwner"`
	RepoName         string `json:"repoName"`
	RepoURL          string `json:"repoUrl"`
	PRTitle          string `json:"prTitle"`
	PRUrl            string `json:"prUrl"`
	MergedAt         string `json:"mergedAt"`
	ContributorCount int    `json:"contributorCount"`
	Stars            int    `json:"stars"`
}
