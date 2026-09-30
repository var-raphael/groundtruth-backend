package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/var-raphael/groundtruth/internal/api/handlers"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/db"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/worker"
	"github.com/var-raphael/groundtruth/pkg/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	ctx := context.Background()

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer pool.Close()

	githubClient := ghextractor.NewClient(cfg.GithubToken)

	mistralClients := make([]*llm.Client, len(cfg.MistralAPIKeys))
	for i, key := range cfg.MistralAPIKeys {
		mistralClients[i] = llm.NewClient(key)
	}

	outreachHandler := &handlers.OutreachHandler{Pool: pool, MistralClient: mistralClients[0]}
	jobsHandler := &handlers.JobsHandler{Pool: pool}
	applyHandler := &handlers.ApplyHandler{Pool: pool}
	candidatesHandler := &handlers.CandidatesHandler{Pool: pool}
	exportHandler := &handlers.ExportHandler{Pool: pool}
	scanHandler := &handlers.ScanHandler{Pool: pool, GithubClient: githubClient, MistralClients: mistralClients}
	shareLinksHandler := &handlers.ShareLinksHandler{Pool: pool}
	publicJobsHandler := &handlers.PublicJobsHandler{Pool: pool, GithubClient: githubClient, DashboardSecret: cfg.DashboardSecret}
	sharedHandler := &handlers.SharedHandler{Pool: pool, MistralClient: mistralClients[0]}

	worker.StartScheduler(ctx, time.Minute, pool, githubClient, mistralClients)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /candidates/{id}/outreach", outreachHandler.GetDraft)
	mux.HandleFunc("POST /candidates/{id}/outreach", outreachHandler.GenerateDraft)
	mux.HandleFunc("PUT /candidates/{id}/outreach", outreachHandler.SaveDraft)
	mux.HandleFunc("POST /jobs", jobsHandler.CreateJob)
	mux.HandleFunc("GET /jobs", jobsHandler.ListJobs)
	mux.HandleFunc("GET /jobs/{id}", jobsHandler.GetJob)
	mux.HandleFunc("PUT /jobs/{id}", jobsHandler.EditJob)
	mux.HandleFunc("PATCH /jobs/{id}", jobsHandler.EditJob)
	mux.HandleFunc("DELETE /jobs/{id}", jobsHandler.DeleteJob)
	mux.HandleFunc("POST /jobs/{id}/apply", applyHandler.Apply)
	mux.HandleFunc("GET /jobs/{id}/candidates", candidatesHandler.ListCandidates)
	mux.HandleFunc("GET /jobs/{id}/reports", candidatesHandler.ListReports)
	mux.HandleFunc("GET /jobs/{id}/export", exportHandler.ExportJob)
	mux.HandleFunc("POST /jobs/{id}/share-link", shareLinksHandler.CreateShareLink)
	mux.HandleFunc("GET /jobs/{id}/share-link", shareLinksHandler.GetShareLink)
	mux.HandleFunc("DELETE /jobs/{id}/share-link/{token}", shareLinksHandler.RevokeShareLink)
	mux.HandleFunc("POST /jobs/public", publicJobsHandler.CreatePublicJob)
	mux.HandleFunc("GET /shared/{token}", sharedHandler.GetShared)
	mux.HandleFunc("POST /shared/{token}/outreach/{candidateId}", sharedHandler.GenerateSharedOutreach)
	mux.HandleFunc("GET /candidates/{id}", candidatesHandler.GetCandidate)
	mux.HandleFunc("GET /candidates/{id}/report", candidatesHandler.GetReport)
	mux.HandleFunc("POST /scan", scanHandler.TriggerScan)
	mux.HandleFunc("POST /candidates/{id}/rescan", scanHandler.RescanByPath)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("listening on :%s", cfg.Port)
	log.Printf("WARNING: using FakeAuth middleware — all requests authenticated as dev recruiter, replace before production")
	if err := http.ListenAndServe(":"+cfg.Port, middleware.CORS(middleware.FakeAuth(mux))); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
