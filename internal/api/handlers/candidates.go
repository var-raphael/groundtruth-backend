package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/timezone"
)

type CandidatesHandler struct {
	Pool *pgxpool.Pool
}

type candidateResponse struct {
	models.Candidate
	OverlapHours int `json:"overlap_hours"`
}

type listReportsResponse struct {
	Reports    []models.CandidateReport `json:"reports"`
	Page       int                      `json:"page"`
	PageSize   int                      `json:"pageSize"`
	Total      int                      `json:"total"`
	TotalPages int                      `json:"totalPages"`

	UnscannedCount int `json:"unscannedCount"`
}

const defaultReportsPageSize = 20
const maxReportsPageSize = 100

func (h *CandidatesHandler) ListReports(w http.ResponseWriter, r *http.Request) {
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

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}
	pageSize := defaultReportsPageSize
	if v := r.URL.Query().Get("pageSize"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= maxReportsPageSize {
			pageSize = parsed
		}
	}

	reports, total, err := queries.ListReportsAndPendingByJob(r.Context(), h.Pool, jobID, pageSize, (page-1)*pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	unscanned, err := queries.CountUnscannedForJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(listReportsResponse{
		Reports:        reports,
		Page:           page,
		PageSize:       pageSize,
		Total:          total,
		TotalPages:     totalPages,
		UnscannedCount: unscanned,
	})
}

func (h *CandidatesHandler) ListCandidates(w http.ResponseWriter, r *http.Request) {
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

	statusFilter := models.CandidateStatus(r.URL.Query().Get("status"))

	candidates, err := queries.ListCandidatesByJob(r.Context(), h.Pool, jobID, statusFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := make([]candidateResponse, len(candidates))
	for i, c := range candidates {
		response[i] = candidateResponse{
			Candidate:    c,
			OverlapHours: timezone.BestOverlapHours(c.Timezone, job.Timezones),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *CandidatesHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	candidateID := r.PathValue("id")

	candidate, err := queries.GetCandidate(r.Context(), h.Pool, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.Error(w, "candidate not found", http.StatusNotFound)
		return
	}

	job, err := queries.GetJob(r.Context(), h.Pool, candidate.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil || job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	report, err := queries.GetReport(r.Context(), h.Pool, candidateID, candidate.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if report == nil {
		http.Error(w, "report not found — candidate may not be scored yet", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (h *CandidatesHandler) GetCandidate(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	candidateID := r.PathValue("id")

	candidate, err := queries.GetCandidate(r.Context(), h.Pool, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.Error(w, "candidate not found", http.StatusNotFound)
		return
	}

	job, err := queries.GetJob(r.Context(), h.Pool, candidate.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil || job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	response := candidateResponse{
		Candidate:    *candidate,
		OverlapHours: timezone.BestOverlapHours(candidate.Timezone, job.Timezones),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
