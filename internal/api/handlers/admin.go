package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/models"
)

const (
	maxAdminBulkCandidates = 25
	adminJobCandidateLimit = 100
)

type AdminHandler struct {
	Pool         *pgxpool.Pool
	GithubClient *github.Client
	AdminEmails  []string
}

func (h *AdminHandler) Guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
			return
		}

		recruiter, err := queries.GetRecruiterByID(r.Context(), h.Pool, recruiterID)
		if err != nil || recruiter == nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
			return
		}

		for _, allowed := range h.AdminEmails {
			if strings.EqualFold(allowed, recruiter.Email) {
				next(w, r)
				return
			}
		}
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
	}
}

func (h *AdminHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := queries.ListShowcaseJobs(r.Context(), h.Pool)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (h *AdminHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	job, err := queries.GetShowcaseJob(r.Context(), h.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

type adminCreateJobRequest struct {
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	Stack              []string            `json:"stack"`
	LocationMode       models.LocationMode `json:"location_mode"`
	LocationCountries  []string            `json:"location_countries"`
	MinYearsExperience int                 `json:"min_years_experience"`
}

func (h *AdminHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	recruiterID, _ := middleware.RecruiterIDFromContext(r.Context())

	var req adminCreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	switch {
	case strings.TrimSpace(req.Title) == "":
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	case strings.TrimSpace(req.Description) == "":
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	case len(req.Stack) == 0:
		http.Error(w, "stack is required", http.StatusBadRequest)
		return
	case req.LocationMode != models.LocationAnywhere && req.LocationMode != models.LocationCountry && req.LocationMode != models.LocationOnsite:
		http.Error(w, "location_mode must be one of: anywhere, country, onsite", http.StatusBadRequest)
		return
	case req.LocationMode != models.LocationAnywhere && len(req.LocationCountries) == 0:
		http.Error(w, "location_countries is required when location_mode is not anywhere", http.StatusBadRequest)
		return
	case req.MinYearsExperience < 0:
		http.Error(w, "min_years_experience must be zero or greater", http.StatusBadRequest)
		return
	}

	countries := req.LocationCountries
	if req.LocationMode == models.LocationAnywhere || countries == nil {
		countries = []string{}
	}

	created, err := queries.CreateJob(r.Context(), h.Pool, &models.Job{
		RecruiterID:        recruiterID,
		Title:              strings.TrimSpace(req.Title),
		Description:        strings.TrimSpace(req.Description),
		Stack:              req.Stack,
		LocationMode:       req.LocationMode,
		LocationCountries:  countries,
		MinYearsExperience: req.MinYearsExperience,
		Timezones:          []string{"UTC"},
		CandidateLimit:     adminJobCandidateLimit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := queries.SetJobShowcase(r.Context(), h.Pool, created.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job, err := queries.GetShowcaseJob(r.Context(), h.Pool, created.ID)
	if err != nil || job == nil {
		http.Error(w, "job created but could not be loaded", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (h *AdminHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	job, err := queries.GetShowcaseJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	if err := queries.DeleteJob(r.Context(), h.Pool, jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) BulkAddCandidates(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	job, err := queries.GetShowcaseJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	var body struct {
		GithubUsernames []string `json:"github_usernames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(body.GithubUsernames) == 0 {
		http.Error(w, "github_usernames is required", http.StatusBadRequest)
		return
	}
	if len(body.GithubUsernames) > maxAdminBulkCandidates {
		http.Error(w, "at most 25 candidates can be added at once", http.StatusBadRequest)
		return
	}

	seen := map[string]bool{}
	queued := 0
	failed := []string{}

	for _, raw := range body.GithubUsernames {
		username := githubUsernameFromURL(raw)
		if username == "" {
			failed = append(failed, raw+": could not parse a username")
			continue
		}
		key := strings.ToLower(username)
		if seen[key] {
			continue
		}
		seen[key] = true

		user, _, err := h.GithubClient.Users.Get(r.Context(), username)
		if err != nil || user.GetID() == 0 {
			failed = append(failed, username+": GitHub user not found")
			continue
		}

		login := user.GetLogin()
		name := user.GetName()
		if name == "" {
			name = login
		}

		_, err = queries.CreateCandidate(r.Context(), h.Pool, &models.Candidate{
			JobID:          jobID,
			FullName:       name,
			Email:          login + "@github.groundtruth.local",
			GithubID:       user.GetID(),
			GithubUsername: login,
			Status:         models.StatusQueued,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				failed = append(failed, login+": already added to this job")
			} else {
				failed = append(failed, login+": could not be added")
			}
			continue
		}
		queued++
	}

	writeJSON(w, http.StatusOK, map[string]any{"queued": queued, "failed": failed})
}
