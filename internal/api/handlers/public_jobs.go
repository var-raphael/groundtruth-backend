package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/models"
)

const maxPublicCandidates = 20

type PublicJobsHandler struct {
	Pool            *pgxpool.Pool
	GithubClient    *github.Client
	DashboardSecret string
}

type publicCandidateInput struct {
	Name      string `json:"name"`
	GithubURL string `json:"github_url"`
	LinkedIn  string `json:"linkedin,omitempty"`
	X         string `json:"x,omitempty"`
	Portfolio string `json:"portfolio,omitempty"`
}

type createPublicJobRequest struct {
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Stack       []string                `json:"stack"`
	Candidates  []publicCandidateInput  `json:"candidates"`
}

func (h *PublicJobsHandler) CreatePublicJob(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Dashboard-Secret") != h.DashboardSecret || h.DashboardSecret == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createPublicJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if req.Description == "" {
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	}
	if len(req.Stack) == 0 {
		http.Error(w, "stack is required", http.StatusBadRequest)
		return
	}
	if len(req.Candidates) == 0 {
		http.Error(w, "candidates is required", http.StatusBadRequest)
		return
	}
	if len(req.Candidates) > maxPublicCandidates {
		http.Error(w, "at most 20 candidates are allowed", http.StatusBadRequest)
		return
	}
	for i, c := range req.Candidates {
		if c.Name == "" {
			http.Error(w, "candidate name is required for every candidate", http.StatusBadRequest)
			return
		}
		if c.GithubURL == "" {
			http.Error(w, "candidate github_url is required for every candidate", http.StatusBadRequest)
			return
		}
		if c.LinkedIn == "" && c.X == "" && c.Portfolio == "" {
			http.Error(w, "each candidate needs at least one of linkedin, x, or portfolio", http.StatusBadRequest)
			return
		}
		req.Candidates[i].GithubURL = strings.TrimSpace(c.GithubURL)
	}

	job := &models.Job{
		RecruiterID:       middleware.DevRecruiterID,
		Title:             req.Title,
		Description:       req.Description,
		Stack:             req.Stack,
		LocationMode:      models.LocationAnywhere,
		LocationCountries: []string{},
		Timezones:         []string{"UTC"},
		CandidateLimit:    maxPublicCandidates,
	}
	createdJob, err := queries.CreateJob(r.Context(), h.Pool, job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	created := make([]*models.Candidate, 0, len(req.Candidates))
	var failed []string
	for _, c := range req.Candidates {
		username := githubUsernameFromURL(c.GithubURL)
		if username == "" {
			failed = append(failed, c.GithubURL+": could not parse a username from this URL")
			continue
		}

		githubID, err := ghextractor.FetchUserID(r.Context(), h.GithubClient, username)
		if err != nil {
			failed = append(failed, username+": "+err.Error())
			continue
		}

		candidate := &models.Candidate{
			JobID:          createdJob.ID,
			FullName:       c.Name,
			Email:          username + "@github.groundtruth.local",
			GithubID:       githubID,
			GithubUsername: username,
			LinkedIn:       nullableString(c.LinkedIn),
			X:              nullableString(c.X),
			Portfolio:      nullableString(c.Portfolio),
			Status:         models.StatusQueued,
		}
		createdCandidate, err := queries.CreateCandidate(r.Context(), h.Pool, candidate)
		if err != nil {
			failed = append(failed, username+": "+err.Error())
			continue
		}
		created = append(created, createdCandidate)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct {
		Job        *models.Job         `json:"job"`
		Candidates []*models.Candidate `json:"candidates"`
		Failed     []string            `json:"failed,omitempty"`
	}{
		Job:        createdJob,
		Candidates: created,
		Failed:     failed,
	})
}

func githubUsernameFromURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.Trim(s, "/")
	if s == "" || strings.Contains(s, "/") {
		return ""
	}
	return s
}
