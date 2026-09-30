package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
)

type ShareLinksHandler struct {
	Pool *pgxpool.Pool
}

func (h *ShareLinksHandler) getOwnedJob(w http.ResponseWriter, r *http.Request, jobID, recruiterID string) bool {
	job, err := queries.GetJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return false
	}
	if job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (h *ShareLinksHandler) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobID := r.PathValue("id")
	if !h.getOwnedJob(w, r, jobID, recruiterID) {
		return
	}

	existing, err := queries.GetActiveJobShareLinkByJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existing)
		return
	}

	link, err := queries.CreateJobShareLink(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

func (h *ShareLinksHandler) GetShareLink(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobID := r.PathValue("id")
	if !h.getOwnedJob(w, r, jobID, recruiterID) {
		return
	}

	link, err := queries.GetActiveJobShareLinkByJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if link == nil {
		http.Error(w, "no active share link for this job", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(link)
}

func (h *ShareLinksHandler) RevokeShareLink(w http.ResponseWriter, r *http.Request) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	jobID := r.PathValue("id")
	if !h.getOwnedJob(w, r, jobID, recruiterID) {
		return
	}

	token := r.PathValue("token")
	revoked, err := queries.RevokeJobShareLink(r.Context(), h.Pool, jobID, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !revoked {
		http.Error(w, "share link not found or already revoked", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
