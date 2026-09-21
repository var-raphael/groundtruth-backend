package models

import "time"

type OutreachDraft struct {
	CandidateID string    `json:"candidateId"`
	JobID       string    `json:"jobId"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	Edited      bool      `json:"edited"`
	Stale       bool      `json:"stale"`
	GeneratedAt time.Time `json:"generatedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
