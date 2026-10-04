package worker

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/google/go-github/v66/github"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/var-raphael/groundtruth/internal/db/queries"
	"github.com/var-raphael/groundtruth/internal/llm"
)

const staleScoringMinutes = 10

const stalePaymentDays = 35

func StartScheduler(ctx context.Context, interval time.Duration, pool *pgxpool.Pool, githubClient *github.Client, mistralClients []*llm.Client) {
	var running int32

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !atomic.CompareAndSwapInt32(&running, 0, 1) {
					log.Printf("scheduler: previous scan still running, skipping this tick")
					continue
				}

				go func() {
					defer atomic.StoreInt32(&running, 0)

					reset, err := queries.ResetStaleScoringCandidates(ctx, pool, staleScoringMinutes)
					if err != nil {
						log.Printf("scheduler: resetting stale candidates: %v", err)
					} else if reset > 0 {
						log.Printf("scheduler: reset %d stale candidate(s) stuck in scoring back to queued", reset)
					}

					expired, expErr := queries.ExpireLapsedPlans(ctx, pool)
					if expErr != nil {
						log.Printf("scheduler: expiring lapsed plans: %v", expErr)
					} else if expired > 0 {
						log.Printf("scheduler: downgraded %d recruiter(s) whose paid period ended", expired)
					}

					unpaid, unpaidErr := queries.ExpireStalePayments(ctx, pool, stalePaymentDays)
					if unpaidErr != nil {
						log.Printf("scheduler: expiring unpaid plans: %v", unpaidErr)
					} else if unpaid > 0 {
						log.Printf("scheduler: downgraded %d recruiter(s) with no payment in %d days", unpaid, stalePaymentDays)
					}

					promoted, err := PromoteUnscanned(ctx, pool)
					if err != nil {
						log.Printf("scheduler: promoting unscanned candidates: %v", err)
					} else if promoted > 0 {
						log.Printf("scheduler: promoted %d unscanned candidate(s) back to the queue", promoted)
					}

					if err := Scan(ctx, pool, githubClient, mistralClients, ScanOptions{}); err != nil {
						log.Printf("scheduler: scan failed: %v", err)
					}
				}()
			}
		}
	}()
}
