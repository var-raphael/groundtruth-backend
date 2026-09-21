package queries

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageStore struct {
	Pool *pgxpool.Pool
}

func (s UsageStore) LastUsageAt(ctx context.Context, candidateID string) (*time.Time, error) {
	return LastUsageAt(ctx, s.Pool, candidateID)
}

func (s UsageStore) CountUsageSince(ctx context.Context, candidateID, action string, since time.Time) (int, time.Time, error) {
	return CountUsageSince(ctx, s.Pool, candidateID, action, since)
}

func (s UsageStore) CountDistinctCandidates(ctx context.Context, recruiterID, action string) (int, error) {
	return CountDistinctCandidates(ctx, s.Pool, recruiterID, action)
}

func (s UsageStore) CandidateHasUsage(ctx context.Context, recruiterID, candidateID, action string) (bool, error) {
	return CandidateHasUsage(ctx, s.Pool, recruiterID, candidateID, action)
}
