package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/outreach"
)

type OutreachHandler struct {
	Pool          *pgxpool.Pool
	MistralClient *llm.Client
}

func (h *OutreachHandler) DraftOutreach(w http.ResponseWriter, r *http.Request) {
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

	draft, err := outreach.BuildDraft(r.Context(), h.MistralClient, report, *job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(draft)
}
