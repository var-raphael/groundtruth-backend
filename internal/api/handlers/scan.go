package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/worker"
)

type ScanHandler struct {
	Pool           *pgxpool.Pool
	GithubClient   *github.Client
	MistralClients []*llm.Client
}

func (h *ScanHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	opts := worker.ScanOptions{
		Force:       r.URL.Query().Get("force") == "true",
		CandidateID: r.URL.Query().Get("candidate_id"),
	}

	go func() {
		if err := worker.Scan(context.Background(), h.Pool, h.GithubClient, h.MistralClients, opts); err != nil {
			log.Printf("scan failed: %v", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("scan started"))
}
