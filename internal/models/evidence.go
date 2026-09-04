package models

// RepoEvidence is one of a candidate's own repos that survived filtering
// (not a bare fork) and was ranked into the top-K considered for scoring.
type RepoEvidence struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Languages   []string `json:"languages"`
	LiveURL     string   `json:"live_url,omitempty"`
	RepoURL     string   `json:"repo_url"`

	Commits90d   int    `json:"commits_90d"`
	LastCommitAt string `json:"last_commit_at"`

	HasReadme    bool `json:"has_readme"`
	ReadmeThin   bool `json:"readme_thin"` // present but too short to count for much

	// authenticity signal from activity.go's pattern check
	SuspiciousPadding bool `json:"suspicious_padding"`
}

// ContributionEvidence is a merged PR on a repo the candidate doesn't own.
type ContributionEvidence struct {
	RepoName         string `json:"repo_name"`
	RepoURL          string `json:"repo_url"`
	ContributorCount int    `json:"contributor_count"`
	Stars            int    `json:"stars"`
	PRUrl            string `json:"pr_url"`
}

// Reason is a single line shown to the recruiter, always tied to evidence.
type Reason struct {
	Point    string   `json:"point"`
	Evidence []string `json:"evidence"` // repo/PR names or URLs backing this claim
}

// JobReasoning is a candidate's score against one specific job.
// Same candidate scores differently per job — this is per (candidate, job).
type JobReasoning struct {
	CandidateID string `json:"candidate_id"`
	JobID       string `json:"job_id"`

	Score      int    `json:"score"` // 0-10
	StackMatch string `json:"stack_match"` // "strong" | "partial" | "weak"

	PositiveReasons []Reason `json:"positive_reasons"` // max 10, ranked by strength
	NegativeReasons []Reason `json:"negative_reasons"` // max 5, ranked by severity

	// dishonesty-type flags (e.g. commit padding) always included in
	// NegativeReasons regardless of ranking — this just marks that one is present
	HasTrustFlag bool `json:"has_trust_flag"`
}
