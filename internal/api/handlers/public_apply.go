package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/var-raphael/groundtruth/internal/auth"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
)

type publicApplyRequest struct {
	FullName        string `json:"full_name"`
	Email           string `json:"email"`
	Country         string `json:"country"`
	City            string `json:"city"`
	YearsExperience int    `json:"years_experience"`
	LinkedIn        string `json:"linkedin"`
	X               string `json:"x"`
	Portfolio       string `json:"portfolio"`
}

type identityResponse struct {
	GithubID       int64  `json:"github_id"`
	GithubUsername string `json:"github_username"`
}

func (h *ApplyHandler) authenticate(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	header := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if header == "" || token == "" || token == header {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
		return nil, false
	}

	claims, err := h.Verifier.Verify(r.Context(), token)
	if err != nil {
		log.Printf("public auth: %v", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "sign in required"})
		return nil, false
	}
	if claims.Provider != "github" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "sign in with GitHub to apply"})
		return nil, false
	}
	return claims, true
}

type githubCheck struct {
	ID       int64
	Login    string
	Rejected bool
	Err      error
}

func checkGithubToken(r *http.Request, token string) githubCheck {
	user, resp, err := ghextractor.NewClient(token).Users.Get(r.Context(), "")
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return githubCheck{Rejected: true}
		}
		return githubCheck{Err: err}
	}
	return githubCheck{ID: user.GetID(), Login: user.GetLogin()}
}

func (h *ApplyHandler) LinkIdentity(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var body struct {
		GithubToken string `json:"github_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GithubToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "github_token is required"})
		return
	}

	check := checkGithubToken(r, body.GithubToken)
	if check.Rejected {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reconnect_github"})
		return
	}
	if check.Err != nil {
		log.Printf("link identity: github check failed: %v", check.Err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach GitHub, try again"})
		return
	}

	err := queries.UpsertCandidateIdentity(r.Context(), h.Pool, queries.CandidateIdentity{
		AuthUserID:     claims.Sub,
		GithubID:       check.ID,
		GithubUsername: check.Login,
		GithubToken:    body.GithubToken,
	})
	if err != nil {
		log.Printf("link identity: %v", err)
		http.Error(w, "could not save GitHub connection", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, identityResponse{GithubID: check.ID, GithubUsername: check.Login})
}

func (h *ApplyHandler) GetIdentity(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	identity, err := queries.GetCandidateIdentity(r.Context(), h.Pool, claims.Sub)
	if err != nil {
		log.Printf("get identity: %v", err)
		http.Error(w, "could not load GitHub connection", http.StatusInternalServerError)
		return
	}
	if identity == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_connected"})
		return
	}

	check := checkGithubToken(r, identity.GithubToken)
	if check.Rejected || (check.Err == nil && check.ID != identity.GithubID) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reconnect_github"})
		return
	}

	writeJSON(w, http.StatusOK, identityResponse{GithubID: identity.GithubID, GithubUsername: identity.GithubUsername})
}

func (h *ApplyHandler) PublicApply(w http.ResponseWriter, r *http.Request) {
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

	claims, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var body publicApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	identity, err := queries.GetCandidateIdentity(r.Context(), h.Pool, claims.Sub)
	if err != nil {
		log.Printf("public apply: %v", err)
		http.Error(w, "could not load GitHub connection", http.StatusInternalServerError)
		return
	}
	if identity == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not_connected"})
		return
	}

	check := checkGithubToken(r, identity.GithubToken)
	if check.Rejected || (check.Err == nil && check.ID != identity.GithubID) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reconnect_github"})
		return
	}

	h.submit(w, r, job, applyRequest{
		FullName:        body.FullName,
		Email:           body.Email,
		Country:         body.Country,
		City:            body.City,
		YearsExperience: body.YearsExperience,
		GithubID:        identity.GithubID,
		GithubUsername:  identity.GithubUsername,
		GithubToken:     identity.GithubToken,
		LinkedIn:        body.LinkedIn,
		X:               body.X,
		Portfolio:       body.Portfolio,
	})
}

func (h *ApplyHandler) ApplicationStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	identity, err := queries.GetCandidateIdentity(r.Context(), h.Pool, claims.Sub)
	if err != nil {
		log.Printf("application status: %v", err)
		http.Error(w, "could not load GitHub connection", http.StatusInternalServerError)
		return
	}
	if identity == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_connected"})
		return
	}

	applied, err := queries.HasApplied(r.Context(), h.Pool, r.PathValue("id"), identity.GithubID, "")
	if err != nil {
		log.Printf("application status: %v", err)
		http.Error(w, "could not check application", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"applied": applied})
}
