package models

import "time"

type LocationMode string

const (
	LocationAnywhere LocationMode = "anywhere"
	LocationCountry  LocationMode = "country"
	LocationOnsite   LocationMode = "onsite"
)

type Job struct {
	ID          string   `json:"id" db:"id"`
	RecruiterID string   `json:"recruiter_id" db:"recruiter_id"`
	Title       string   `json:"title" db:"title"`
	Description string   `json:"description" db:"description"`
	Stack       []string `json:"stack" db:"stack"`

	LocationMode      LocationMode `json:"location_mode" db:"location_mode"`
	LocationCountries []string     `json:"location_countries" db:"location_countries"`

	MinYearsExperience int `json:"min_years_experience" db:"min_years_experience"`

	Timezones       []string `json:"timezones" db:"timezones"`
	MinOverlapHours int      `json:"min_overlap_hours" db:"min_overlap_hours"`

	CandidateLimit int `json:"candidate_limit" db:"candidate_limit"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
