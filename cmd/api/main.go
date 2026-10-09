package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/var-raphael/groundtruth/internal/api/handlers"
	"github.com/var-raphael/groundtruth/internal/api/middleware"
	"github.com/var-raphael/groundtruth/internal/auth"
	"github.com/var-raphael/groundtruth/internal/db"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/paystack"
	"github.com/var-raphael/groundtruth/internal/secrets"
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

	if err := secrets.Init(cfg.TokenEncryptionKey); err != nil {
		log.Fatalf("%v", err)
	}

	githubClient := ghextractor.NewClient(cfg.GithubToken)

	mistralClients := make([]*llm.Client, len(cfg.MistralAPIKeys))
	for i, key := range cfg.MistralAPIKeys {
		mistralClients[i] = llm.NewClient(key)
	}

	outreachHandler := &handlers.OutreachHandler{Pool: pool, MistralClient: mistralClients[0]}
	jobsHandler := &handlers.JobsHandler{Pool: pool}
	verifier := auth.NewVerifier(cfg.SupabaseURL)
	applyHandler := &handlers.ApplyHandler{Pool: pool, Verifier: verifier}
	candidatesHandler := &handlers.CandidatesHandler{Pool: pool}
	exportHandler := &handlers.ExportHandler{Pool: pool}
	scanHandler := &handlers.ScanHandler{Pool: pool, GithubClient: githubClient, MistralClients: mistralClients}
	shareLinksHandler := &handlers.ShareLinksHandler{Pool: pool}
	publicJobsHandler := &handlers.PublicJobsHandler{Pool: pool, GithubClient: githubClient, DashboardSecret: cfg.DashboardSecret}
	sharedHandler := &handlers.SharedHandler{Pool: pool, MistralClient: mistralClients[0]}
	billingHandler := &handlers.BillingHandler{
		Pool:        pool,
		Paystack:    paystack.NewClient(cfg.PaystackSecretKey),
		ProPlanCode: cfg.PaystackProPlanCode,
		AppURL:      cfg.AppURL,
	}
	adminHandler := &handlers.AdminHandler{Pool: pool, GithubClient: githubClient, AdminEmails: cfg.AdminEmails}
	showcaseHandler := &handlers.ShowcaseHandler{Pool: pool, MistralClient: mistralClients[0]}

	worker.StartScheduler(ctx, time.Minute, pool, githubClient, mistralClients)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /candidates/{id}/outreach", outreachHandler.GetDraft)
	mux.HandleFunc("POST /candidates/{id}/outreach", outreachHandler.GenerateDraft)
	mux.HandleFunc("PUT /candidates/{id}/outreach", outreachHandler.SaveDraft)
	mux.HandleFunc("POST /jobs", jobsHandler.CreateJob)
	mux.HandleFunc("GET /jobs", jobsHandler.ListJobs)
	mux.HandleFunc("GET /me/plan", jobsHandler.PlanInfo)
	mux.HandleFunc("GET /jobs/{id}", jobsHandler.GetJob)
	mux.HandleFunc("PUT /jobs/{id}", jobsHandler.EditJob)
	mux.HandleFunc("PATCH /jobs/{id}", jobsHandler.EditJob)
	mux.HandleFunc("DELETE /jobs/{id}", jobsHandler.DeleteJob)
	mux.HandleFunc("POST /jobs/{id}/apply", applyHandler.Apply)
	mux.HandleFunc("GET /public/jobs/{id}", applyHandler.PublicJob)
	mux.HandleFunc("GET /public/jobs/{id}/report", showcaseHandler.Report)
	mux.HandleFunc("POST /public/candidates/{candidateId}/outreach", showcaseHandler.Outreach)
	mux.HandleFunc("GET /public/pricing", billingHandler.Pricing)
	mux.HandleFunc("POST /billing/checkout", billingHandler.Checkout)
	mux.HandleFunc("POST /billing/cancel", billingHandler.Cancel)
	mux.HandleFunc("POST /webhooks/paystack", billingHandler.Webhook)
	mux.HandleFunc("GET /admin/jobs", adminHandler.Guard(adminHandler.ListJobs))
	mux.HandleFunc("POST /admin/jobs", adminHandler.Guard(adminHandler.CreateJob))
	mux.HandleFunc("GET /admin/jobs/{id}", adminHandler.Guard(adminHandler.GetJob))
	mux.HandleFunc("DELETE /admin/jobs/{id}", adminHandler.Guard(adminHandler.DeleteJob))
	mux.HandleFunc("POST /admin/jobs/{id}/candidates/bulk", adminHandler.Guard(adminHandler.BulkAddCandidates))
	mux.HandleFunc("GET /public/identity", applyHandler.GetIdentity)
	mux.HandleFunc("POST /public/identity", applyHandler.LinkIdentity)
	mux.HandleFunc("POST /public/jobs/{id}/apply", applyHandler.PublicApply)
	mux.HandleFunc("GET /public/jobs/{id}/application", applyHandler.ApplicationStatus)
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
	
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		http.Error(w, "db unreachable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
})

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, middleware.CORS(middleware.RecruiterAuth(verifier, pool, mux))); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
