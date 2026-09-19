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
	scanHandler := &handlers.ScanHandler{Pool: pool, GithubClient: githubClient, MistralClients: mistralClients}

	worker.StartScheduler(ctx, time.Minute, pool, githubClient, mistralClients)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /candidates/{id}/outreach", outreachHandler.DraftOutreach)
	mux.HandleFunc("POST /jobs", jobsHandler.CreateJob)
	mux.HandleFunc("GET /jobs", jobsHandler.ListJobs)
	mux.HandleFunc("GET /jobs/{id}", jobsHandler.GetJob)
	mux.HandleFunc("DELETE /jobs/{id}", jobsHandler.DeleteJob)
	mux.HandleFunc("POST /jobs/{id}/apply", applyHandler.Apply)
	mux.HandleFunc("GET /jobs/{id}/candidates", candidatesHandler.ListCandidates)
	mux.HandleFunc("GET /jobs/{id}/reports", candidatesHandler.ListReports)
	mux.HandleFunc("GET /candidates/{id}", candidatesHandler.GetCandidate)
	mux.HandleFunc("GET /candidates/{id}/report", candidatesHandler.GetReport)
	mux.HandleFunc("POST /scan", scanHandler.TriggerScan)
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
