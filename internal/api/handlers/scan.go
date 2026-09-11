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

// TriggerScan kicks off a worker.Scan run in the background and returns
// immediately. There's no job queue/scheduler yet, so this is a manual
// trigger; a real deployment would run Scan on a timer instead. The scan
// runs against context.Background(), not the request's context, since the
// request context is cancelled as soon as this handler returns.
func (h *ScanHandler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := worker.Scan(context.Background(), h.Pool, h.GithubClient, h.MistralClients); err != nil {
			log.Printf("scan failed: %v", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("scan started"))
}
