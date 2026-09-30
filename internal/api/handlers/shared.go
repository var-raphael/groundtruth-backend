package handlers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/outreach"
)

const maxShareLinkOutreachCandidates = 3

type SharedHandler struct {
	Pool          *pgxpool.Pool
	MistralClient *llm.Client
}

type sharedCandidateView struct {
	CandidateID    string                       `json:"candidateId"`
	Name           string                       `json:"name"`
	GithubUsername string                       `json:"githubUsername"`
	LinkedIn       string                       `json:"linkedin,omitempty"`
	X              string                       `json:"x,omitempty"`
	Portfolio      string                       `json:"portfolio,omitempty"`
	Status         string                       `json:"status"`
	Pending        bool                         `json:"pending,omitempty"`
	Evidence       []models.RepoEvidenceSummary `json:"evidence,omitempty"`
	Contributions  []models.ContributionSummary `json:"contributions,omitempty"`
	Reasoning      *models.ReasoningSummary     `json:"reasoning,omitempty"`
	Warning        string                       `json:"warning,omitempty"`
}

type sharedJobView struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Stack       []string `json:"stack"`
}

type sharedViewResponse struct {
	Job               sharedJobView         `json:"job"`
	Candidates        []sharedCandidateView `json:"candidates"`
	OutreachRemaining int                   `json:"outreachRemaining"`
}

func (h *SharedHandler) resolveActiveLink(w http.ResponseWriter, r *http.Request) *queries.JobShareLink {
	token := r.PathValue("token")
	link, err := queries.GetJobShareLinkByToken(r.Context(), h.Pool, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	if link == nil || link.RevokedAt != nil {
		http.Error(w, "share link not found", http.StatusNotFound)
		return nil
	}
	return link
}

func (h *SharedHandler) GetShared(w http.ResponseWriter, r *http.Request) {
	link := h.resolveActiveLink(w, r)
	if link == nil {
		return
	}

	job, err := queries.GetJob(r.Context(), h.Pool, link.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	reports, _, err := queries.ListReportsAndPendingByJob(r.Context(), h.Pool, link.JobID, 200, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	used, err := queries.CountDistinctShareLinkOutreachCandidates(r.Context(), h.Pool, link.Token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	remaining := maxShareLinkOutreachCandidates - used
	if remaining < 0 {
		remaining = 0
	}

	candidates := make([]sharedCandidateView, 0, len(reports))
	for _, rep := range reports {
		view := sharedCandidateView{
			CandidateID:    rep.CandidateID,
			Name:           rep.Candidate.Name,
			GithubUsername: rep.Candidate.GithubUsername,
			LinkedIn:       rep.Candidate.LinkedIn,
			X:              rep.Candidate.X,
			Portfolio:      rep.Candidate.Portfolio,
			Status:         rep.Candidate.Status,
			Pending:        rep.Pending,
		}
		if !rep.Pending {
			view.Evidence = rep.Evidence
			view.Contributions = rep.Contributions
			reasoning := rep.Reasoning
			view.Reasoning = &reasoning
			view.Warning = rep.Warning
		}
		candidates = append(candidates, view)
	}

	writeJSON(w, http.StatusOK, sharedViewResponse{
		Job: sharedJobView{
			Title:       job.Title,
			Description: job.Description,
			Stack:       job.Stack,
		},
		Candidates:        candidates,
		OutreachRemaining: remaining,
	})
}

func (h *SharedHandler) GenerateSharedOutreach(w http.ResponseWriter, r *http.Request) {
	link := h.resolveActiveLink(w, r)
	if link == nil {
		return
	}

	candidateID := r.PathValue("candidateId")
	candidate, err := queries.GetCandidate(r.Context(), h.Pool, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidate == nil || candidate.JobID != link.JobID {
		http.Error(w, "candidate not found", http.StatusNotFound)
		return
	}

	alreadyUsed, err := queries.ShareLinkHasOutreachForCandidate(r.Context(), h.Pool, link.Token, candidateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !alreadyUsed {
		used, err := queries.CountDistinctShareLinkOutreachCandidates(r.Context(), h.Pool, link.Token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if used >= maxShareLinkOutreachCandidates {
			writeJSON(w, http.StatusForbidden, deniedResponse{
				Error:  "this share link has already generated outreach for 3 candidates",
				Reason: "lifetime_limit",
			})
			return
		}
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

	job, err := queries.GetJob(r.Context(), h.Pool, link.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	report, err := queries.GetReport(r.Context(), h.Pool, candidateID, link.JobID)
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

	if err := queries.SaveGeneratedOutreachDraft(r.Context(), h.Pool, candidateID, link.JobID, draft.Subject, draft.Body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := queries.RecordShareLinkOutreach(r.Context(), h.Pool, link.Token, candidateID); err != nil {
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
