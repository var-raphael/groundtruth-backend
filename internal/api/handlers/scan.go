package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/plans"
	"github.com/var-raphael/groundtruth/internal/worker"
)

type ScanHandler struct {
	Pool           *pgxpool.Pool
	GithubClient   *github.Client
	MistralClients []*llm.Client
}

func (h *ScanHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	candidateID := r.URL.Query().Get("candidate_id")
	force := r.URL.Query().Get("force") == "true"

	if candidateID == "" {
		go func() {
			if err := worker.Scan(context.Background(), h.Pool, h.GithubClient, h.MistralClients, worker.ScanOptions{}); err != nil {
				log.Printf("scan failed: %v", err)
			}
		}()
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("scan started"))
		return
	}

	h.rescanCandidate(w, r, candidateID, force)
}

func (h *ScanHandler) rescanCandidate(w http.ResponseWriter, r *http.Request, candidateID string, force bool) {
	recruiterID, ok := middleware.RecruiterIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

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

	if candidate.Status == models.StatusExtracting || candidate.Status == models.StatusScoring {
		http.Error(w, "this candidate is already being scanned", http.StatusConflict)
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

	decision, err := plans.Check(r.Context(), queries.UsageStore{Pool: h.Pool}, plans.For(recruiter.Plan), recruiterID, candidateID, plans.ActionRescan, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !decision.Allowed {
		writeDenied(w, decision)
		return
	}

	opts := worker.ScanOptions{Force: force, CandidateID: candidateID}
	go func() {
		ctx := context.Background()
		if err := worker.Scan(ctx, h.Pool, h.GithubClient, h.MistralClients, opts); err != nil {
			log.Printf("rescan failed for candidate %s: %v", candidateID, err)
			return
		}
		after, err := queries.GetCandidate(ctx, h.Pool, candidateID)
		if err != nil || after == nil || after.Status != models.StatusScored {
			return
		}
		if err := queries.RecordUsage(ctx, h.Pool, recruiterID, candidateID, plans.ActionRescan); err != nil {
			log.Printf("recording rescan usage for candidate %s: %v", candidateID, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("rescan started"))
}
