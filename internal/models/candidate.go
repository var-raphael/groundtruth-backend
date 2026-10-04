package models

import "time"

type Candidate struct {
	ID    string `json:"id" db:"id"`
	JobID string `json:"job_id" db:"job_id"`

	FullName string `json:"full_name" db:"full_name"`
	Email    string `json:"email" db:"email"`
	Country  string `json:"country" db:"country"`
	City     string `json:"city" db:"city"`
	Timezone string `json:"timezone" db:"timezone"`

	YearsExperience int `json:"years_experience" db:"years_experience"`

	GithubID       int64   `json:"github_id" db:"github_id"`
	GithubUsername string  `json:"github_username" db:"github_username"`
	GithubToken    *string `json:"-" db:"github_token"` // encrypted at rest, never serialized out

	LinkedIn  *string `json:"linkedin,omitempty" db:"linkedin"`
	X         *string `json:"x,omitempty" db:"x"`
	Portfolio *string `json:"portfolio,omitempty" db:"portfolio"`

	Status          CandidateStatus `json:"status" db:"status"`
	StatusUpdatedAt time.Time       `json:"status_updated_at" db:"status_updated_at"`

	AppliedAt time.Time `json:"applied_at" db:"applied_at"`
}

type CandidateStatus string

const (
	StatusQueued    CandidateStatus = "queued"     // applied, not yet processed (e.g. over plan limit)
	StatusExtracting CandidateStatus = "extracting" // github extraction in progress
	StatusExtracted CandidateStatus = "extracted"  // github evidence saved, LLM scoring not yet run
	StatusScoring   CandidateStatus = "scoring"     // worker actively running the LLM scoring step
	StatusScored    CandidateStatus = "scored"      // has a JobReasoning result
	StatusFailed    CandidateStatus = "failed"      // pipeline errored, needs retry/investigation
	StatusUnscanned CandidateStatus = "unscanned"   // over plan limit, accepted but not scanned until the plan allows it
)

func (c Candidate) ToSummary() CandidateSummary {
	return CandidateSummary{
		CandidateID:            c.ID,
		Name:                   c.FullName,
		GithubID:               c.GithubID,
		GithubUsername:         c.GithubUsername,
		Email:                  c.Email,
		Country:                c.Country,
		City:                   c.City,
		Timezone:               c.Timezone,
		ClaimedExperienceYears: c.YearsExperience,
		LinkedIn:               strPtrOrEmpty(c.LinkedIn),
		X:                      strPtrOrEmpty(c.X),
		Portfolio:              strPtrOrEmpty(c.Portfolio),
		Status:                 string(c.Status),
		StatusUpdatedAt:        c.StatusUpdatedAt,
		AppliedAt:              c.AppliedAt,
	}
}

func strPtrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
