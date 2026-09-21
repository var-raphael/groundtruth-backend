package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/outreach"
	"github.com/var-raphael/groundtruth/internal/plans"
)

type OutreachHandler struct {
	Pool          *pgxpool.Pool
	MistralClient *llm.Client
}

type saveDraftRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type deniedResponse struct {
	Error             string `json:"error"`
	Reason            string `json:"reason"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
}

func (h *OutreachHandler) authorize(w http.ResponseWriter, r *http.Request) (recruiterID string, candidate *models.Candidate, job *models.Job, ok bool) {
	recruiterID, found := middleware.RecruiterIDFromContext(r.Context())
	if !found {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", nil, nil, false
	}

	candidate, err := queries.GetCandidate(r.Context(), h.Pool, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", nil, nil, false
	}
	if candidate == nil {
		http.Error(w, "candidate not found", http.StatusNotFound)
		return "", nil, nil, false
	}

	job, err = queries.GetJob(r.Context(), h.Pool, candidate.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return "", nil, nil, false
	}
	if job == nil || job.RecruiterID != recruiterID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return "", nil, nil, false
	}

	return recruiterID, candidate, job, true
}

func (h *OutreachHandler) GetDraft(w http.ResponseWriter, r *http.Request) {
	_, candidate, _, ok := h.authorize(w, r)
	if !ok {
		return
	}

	draft, err := queries.GetOutreachDraft(r.Context(), h.Pool, candidate.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if draft == nil {
		http.Error(w, "no draft saved for this candidate", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, draft)
}

func (h *OutreachHandler) GenerateDraft(w http.ResponseWriter, r *http.Request) {
	recruiterID, candidate, job, ok := h.authorize(w, r)
	if !ok {
		return
	}

	regenerate := r.URL.Query().Get("regenerate") == "true"

	existing, err := queries.GetOutreachDraft(r.Context(), h.Pool, candidate.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing != nil && !regenerate {
		writeJSON(w, http.StatusOK, existing)
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

	decision, err := plans.Check(r.Context(), queries.UsageStore{Pool: h.Pool}, plans.For(recruiter.Plan), recruiterID, candidate.ID, plans.ActionOutreach, time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !decision.Allowed {
		writeDenied(w, decision)
		return
	}

	report, err := queries.GetReport(r.Context(), h.Pool, candidate.ID, candidate.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if report == nil {
		http.Error(w, "report not found, candidate may not be scored yet", http.StatusNotFound)
		return
	}

	draft, err := outreach.BuildDraft(r.Context(), h.MistralClient, report, *job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := queries.SaveGeneratedOutreachDraft(r.Context(), h.Pool, candidate.ID, candidate.JobID, draft.Subject, draft.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := queries.RecordUsage(r.Context(), h.Pool, recruiterID, candidate.ID, plans.ActionOutreach); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	saved, err := queries.GetOutreachDraft(r.Context(), h.Pool, candidate.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status := http.StatusOK
	if existing == nil {
		status = http.StatusCreated
	}
	writeJSON(w, status, saved)
}

func (h *OutreachHandler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	_, candidate, _, ok := h.authorize(w, r)
	if !ok {
		return
	}

	var req saveDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.Body = strings.TrimSpace(req.Body)
	if req.Subject == "" || req.Body == "" {
		http.Error(w, "subject and body are required", http.StatusBadRequest)
		return
	}

	updated, err := queries.SaveEditedOutreachDraft(r.Context(), h.Pool, candidate.ID, req.Subject, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !updated {
		http.Error(w, "no draft to edit, generate one first", http.StatusNotFound)
		return
	}

	saved, err := queries.GetOutreachDraft(r.Context(), h.Pool, candidate.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func writeDenied(w http.ResponseWriter, d plans.Decision) {
	status := http.StatusForbidden
	if d.Reason == plans.DenyThrottled || d.Reason == plans.DenyDailyLimit {
		status = http.StatusTooManyRequests
	}
	resp := deniedResponse{Error: d.Message, Reason: string(d.Reason)}
	if d.RetryAfter > 0 {
		seconds := int(d.RetryAfter.Seconds()) + 1
		resp.RetryAfterSeconds = seconds
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("outreach: writing response: %v", err)
	}
}
