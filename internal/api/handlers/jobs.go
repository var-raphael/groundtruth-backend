package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/plans"
)

type JobsHandler struct {
	Pool *pgxpool.Pool
}

type createJobRequest struct {
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	Stack              []string            `json:"stack"`
	LocationMode       models.LocationMode `json:"location_mode"`
	LocationCountries  []string            `json:"location_countries"`
	MinYearsExperience int                 `json:"min_years_experience"`
	Timezones          []string            `json:"timezones"`
	MinOverlapHours    int                 `json:"min_overlap_hours"`
}

func validateJobRequest(req createJobRequest) string {
	if req.Title == "" {
		return "title is required"
	}
	if req.Description == "" {
		return "description is required"
	}
	if len(req.Stack) == 0 {
		return "stack is required"
	}
	if req.LocationMode == "" {
		return "location_mode is required"
	}
	if req.LocationMode != models.LocationAnywhere && req.LocationMode != models.LocationCountry && req.LocationMode != models.LocationOnsite {
		return "location_mode must be one of: anywhere, country, onsite"
	}
	if req.LocationMode != models.LocationAnywhere && len(req.LocationCountries) == 0 {
		return "location_countries is required when location_mode is not anywhere"
	}
	if req.MinYearsExperience < 0 {
		return "min_years_experience must be zero or greater"
	}
	if len(req.Timezones) == 0 {
		return "timezones is required"
	}
	if req.MinOverlapHours < 0 {
		return "min_overlap_hours must be zero or greater"
	}
	return ""
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
	if msg := validateJobRequest(req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.LocationMode == models.LocationAnywhere {
		req.LocationCountries = []string{}
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

	plan := plans.For(recruiter.Plan)

	if plan.MaxJobs != plans.Unlimited {
		existing, err := queries.ListJobsByRecruiter(r.Context(), h.Pool, recruiterID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(existing) >= plan.MaxJobs {
			http.Error(w, fmt.Sprintf("your %s plan allows %d job(s), upgrade to create more", plan.Name, plan.MaxJobs), http.StatusForbidden)
			return
		}
	}

	limit := plan.MaxCandidatesPerJob

	job := &models.Job{
		RecruiterID:        recruiterID,
		Title:              req.Title,
		Description:        req.Description,
		Stack:              req.Stack,
		LocationMode:       req.LocationMode,
		LocationCountries:  req.LocationCountries,
		MinYearsExperience: req.MinYearsExperience,
		CandidateLimit:     limit,
		Timezones:          req.Timezones,
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

func (h *JobsHandler) EditJob(w http.ResponseWriter, r *http.Request) {
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

	count, err := queries.CountCandidatesForJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "this job already has candidates and can no longer be edited, create a new job instead", http.StatusConflict)
		return
	}

	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if msg := validateJobRequest(req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if req.LocationMode == models.LocationAnywhere {
		req.LocationCountries = []string{}
	}

	job.Title = req.Title
	job.Description = req.Description
	job.Stack = req.Stack
	job.LocationMode = req.LocationMode
	job.LocationCountries = req.LocationCountries
	job.MinYearsExperience = req.MinYearsExperience
	job.Timezones = req.Timezones
	job.MinOverlapHours = req.MinOverlapHours

	if err := queries.UpdateJob(r.Context(), h.Pool, job); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
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

type deleteConfirmationResponse struct {
	Error          string `json:"error"`
	ConfirmToken   string `json:"confirmToken"`
	CandidateCount int    `json:"candidateCount"`
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

	count, err := queries.CountCandidatesForJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if count == 0 {
		if err := queries.DeleteJob(r.Context(), h.Pool, jobID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	token := r.URL.Query().Get("confirm_token")
	if token == "" {
		newToken, err := queries.CreateJobDeleteConfirmation(r.Context(), h.Pool, jobID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(deleteConfirmationResponse{
			Error:          "this job has candidates, download the full report before deleting",
			ConfirmToken:   newToken,
			CandidateCount: count,
		})
		return
	}

	confirmed, err := queries.ConsumeJobDeleteConfirmation(r.Context(), h.Pool, jobID, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !confirmed {
		http.Error(w, "confirm_token is invalid, expired, or the report has not been downloaded yet", http.StatusPreconditionFailed)
		return
	}

	if err := queries.DeleteJob(r.Context(), h.Pool, jobID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *JobsHandler) PlanInfo(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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

	plan := plans.For(recruiter.Plan)

	var planExpiresAt any
	hasSubscription := false
	if billing, err := queries.GetBilling(r.Context(), h.Pool, recruiterID); err == nil && billing != nil {
		if billing.PlanExpiresAt != nil {
			planExpiresAt = billing.PlanExpiresAt
		}
		hasSubscription = billing.SubscriptionCode != ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"plan":                recruiter.Plan,
		"maxJobs":             plan.MaxJobs,
		"maxCandidatesPerJob": plan.MaxCandidatesPerJob,
		"exportFormats":       plan.ExportFormats,
		"planExpiresAt":       planExpiresAt,
		"hasSubscription":     hasSubscription,
	})
}
