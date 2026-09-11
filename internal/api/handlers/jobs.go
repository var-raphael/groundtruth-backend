package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/models"
)

type JobsHandler struct {
	Pool *pgxpool.Pool
}

var planCandidateLimits = map[string]int{
	"free": 50,
	"pro":  500,
}

type createJobRequest struct {
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	Stack              []string            `json:"stack"`
	LocationMode       models.LocationMode `json:"location_mode"`
	LocationCountries  []string            `json:"location_countries"`
	MinYearsExperience int                 `json:"min_years_experience"`
	Timezone           string              `json:"timezone"`
	MinOverlapHours    int                 `json:"min_overlap_hours"`
}

func (h *JobsHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createJobRequest
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
	if req.LocationMode == "" {
		http.Error(w, "location_mode is required", http.StatusBadRequest)
		return
	}
	if req.LocationMode != models.LocationAnywhere && req.LocationMode != models.LocationCountry && req.LocationMode != models.LocationOnsite {
		http.Error(w, "location_mode must be one of: anywhere, country, onsite", http.StatusBadRequest)
		return
	}
	if len(req.LocationCountries) == 0 {
		http.Error(w, "location_countries is required", http.StatusBadRequest)
		return
	}
	if req.MinYearsExperience < 0 {
		http.Error(w, "min_years_experience must be zero or greater", http.StatusBadRequest)
		return
	}
	if req.Timezone == "" {
		http.Error(w, "timezone is required", http.StatusBadRequest)
		return
	}
	if req.MinOverlapHours < 0 {
		http.Error(w, "min_overlap_hours must be zero or greater", http.StatusBadRequest)
		return
	}

	recruiter, err := queries.GetRecruiterByID(r.Context(), h.Pool, recruiterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if recruiter == nil {
		http.Error(w, "recruiter not found", http.StatusUnauthorized)
		return
	}

	limit, ok := planCandidateLimits[recruiter.Plan]
	if !ok {
		limit = planCandidateLimits["free"]
	}

	job := &models.Job{
		RecruiterID:        recruiterID,
		Title:              req.Title,
		Description:        req.Description,
		Stack:              req.Stack,
		LocationMode:       req.LocationMode,
		LocationCountries:  req.LocationCountries,
		MinYearsExperience: req.MinYearsExperience,
		CandidateLimit:     limit,
		Timezone:           req.Timezone,
		MinOverlapHours:    req.MinOverlapHours,
	}
	created, err := queries.CreateJob(r.Context(), h.Pool, job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (h *JobsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobs, err := queries.ListJobsByRecruiter(r.Context(), h.Pool, recruiterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (h *JobsHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

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
	if job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (h *JobsHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

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
	if job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := queries.DeleteJob(r.Context(), h.Pool, jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
