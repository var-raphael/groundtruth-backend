package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/auth"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/timezone"
)

type ApplyHandler struct {
	Pool     *pgxpool.Pool
	Verifier *auth.Verifier
}

type applyRequest struct {
	FullName        string `json:"full_name"`
	Email           string `json:"email"`
	Country         string `json:"country"`
	City            string `json:"city"`
	YearsExperience int    `json:"years_experience"`
	GithubID        int64  `json:"github_id"`
	GithubUsername  string `json:"github_username"`
	GithubToken     string `json:"github_token"`
	LinkedIn        string `json:"linkedin"`
	X               string `json:"x"`
	Portfolio       string `json:"portfolio"`
}

func (h *ApplyHandler) Apply(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	job, err := queries.GetJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	var req applyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	h.submit(w, r, job, req)
}

func (h *ApplyHandler) submit(w http.ResponseWriter, r *http.Request, job *models.Job, req applyRequest) {
	jobID := job.ID

	if req.FullName == "" {
		http.Error(w, "full_name is required", http.StatusBadRequest)
		return
	}
	if req.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}
	if req.Country == "" {
		http.Error(w, "country is required", http.StatusBadRequest)
		return
	}
	if req.City == "" {
		http.Error(w, "city is required", http.StatusBadRequest)
		return
	}
	if req.YearsExperience < 0 {
		http.Error(w, "years_experience must be zero or greater", http.StatusBadRequest)
		return
	}
	if req.GithubID == 0 {
		http.Error(w, "github_id is required", http.StatusBadRequest)
		return
	}
	if req.GithubUsername == "" {
		http.Error(w, "github_username is required", http.StatusBadRequest)
		return
	}
	if req.LinkedIn == "" && req.X == "" && req.Portfolio == "" {
		http.Error(w, "at least one of linkedin, x, or portfolio is required", http.StatusBadRequest)
		return
	}

	applied, err := queries.HasApplied(r.Context(), h.Pool, jobID, req.GithubID, req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if applied {
		http.Error(w, "you have already applied to this job", http.StatusConflict)
		return
	}

	count, err := queries.CountCandidatesForJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status := models.StatusQueued
	if count >= job.CandidateLimit {
		status = models.StatusUnscanned
	}

	tz := timezone.Derive(req.City)

	candidate := &models.Candidate{
		JobID:           jobID,
		FullName:        req.FullName,
		Email:           req.Email,
		Country:         req.Country,
		City:            req.City,
		Timezone:        tz,
		YearsExperience: req.YearsExperience,
		GithubID:        req.GithubID,
		GithubUsername:  req.GithubUsername,
		GithubToken:     nullableString(req.GithubToken),
		LinkedIn:        nullableString(req.LinkedIn),
		X:               nullableString(req.X),
		Portfolio:       nullableString(req.Portfolio),
		Status:          status,
	}

	created, err := queries.CreateCandidate(r.Context(), h.Pool, candidate)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, "you have already applied to this job", http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type publicJobResponse struct {
	ID                string              `json:"id"`
	Title             string              `json:"title"`
	Description       string              `json:"description"`
	Stack             []string            `json:"stack"`
	LocationMode      models.LocationMode `json:"location_mode"`
	LocationCountries []string            `json:"location_countries"`
}

func (h *ApplyHandler) PublicJob(w http.ResponseWriter, r *http.Request) {
	job, err := queries.GetJob(r.Context(), h.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(publicJobResponse{
		ID:                job.ID,
		Title:             job.Title,
		Description:       job.Description,
		Stack:             job.Stack,
		LocationMode:      job.LocationMode,
		LocationCountries: job.LocationCountries,
	})
}
