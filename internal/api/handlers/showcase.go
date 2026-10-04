package handlers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/outreach"
)

type ShowcaseHandler struct {
	Pool          *pgxpool.Pool
	MistralClient *llm.Client
}

type showcaseJobView struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	Description        string              `json:"description"`
	Stack              []string            `json:"stack"`
	LocationMode       models.LocationMode `json:"location_mode"`
	LocationCountries  []string            `json:"location_countries"`
	MinYearsExperience int                 `json:"min_years_experience"`
}

func (h *ShowcaseHandler) Report(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")

	showcase, err := queries.IsShowcaseJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !showcase {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	job, err := queries.GetJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "report not found", http.StatusNotFound)
		return
	}

	reports, err := queries.AllReportsByJob(r.Context(), h.Pool, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reports == nil {
		reports = []models.CandidateReport{}
	}
	for i := range reports {
		reports[i].Candidate.Email = ""
	}

	stack := job.Stack
	if stack == nil {
		stack = []string{}
	}
	countries := job.LocationCountries
	if countries == nil {
		countries = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job": showcaseJobView{
			ID:                 job.ID,
			Title:              job.Title,
			Description:        job.Description,
			Stack:              stack,
			LocationMode:       job.LocationMode,
			LocationCountries:  countries,
			MinYearsExperience: job.MinYearsExperience,
		},
		"reports": reports,
	})
}

func (h *ShowcaseHandler) Outreach(w http.ResponseWriter, r *http.Request) {
	candidateID := r.PathValue("candidateId")

	candidate, err := queries.GetCandidate(r.Context(), h.Pool, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil {
		http.Error(w, "candidate not found", http.StatusNotFound)
		return
	}

	showcase, err := queries.IsShowcaseJob(r.Context(), h.Pool, candidate.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !showcase {
		http.Error(w, "candidate not found", http.StatusNotFound)
		return
	}

	existing, err := queries.GetOutreachDraft(r.Context(), h.Pool, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing != nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}

	job, err := queries.GetJob(r.Context(), h.Pool, candidate.JobID)
	if err != nil || job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	report, err := queries.GetReport(r.Context(), h.Pool, candidateID, candidate.JobID)
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
		writeDraftError(w, err)
		return
	}

	if err := queries.SaveGeneratedOutreachDraft(r.Context(), h.Pool, candidateID, candidate.JobID, draft.Subject, draft.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	saved, err := queries.GetOutreachDraft(r.Context(), h.Pool, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}
