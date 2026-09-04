package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
	"github.com/var-raphael/groundtruth/internal/outreach"
)

type OutreachHandler struct {
	MistralClient *llm.Client
}

type outreachRequest struct {
	Report models.CandidateReport `json:"report"`
	Job    models.Job             `json:"job"`
}

func (h *OutreachHandler) DraftOutreach(w http.ResponseWriter, r *http.Request) {
	var req outreachRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	draft, err := outreach.BuildDraft(r.Context(), h.MistralClient, &req.Report, req.Job)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(draft)
}
