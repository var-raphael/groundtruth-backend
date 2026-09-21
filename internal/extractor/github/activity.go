package github

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/go-github/v66/github"
)

type WeeklyCommits struct {
	WeekStart int64
	Total     int
}

type ActivitySummary struct {
	TotalCommits90d int
	ActiveWeeks90d  int

	SuspiciousPadding bool
}

const (
	activityStatsMaxRetries = 3
	activityStatsRetryWait  = 5 * time.Second
)

func FetchActivity(ctx context.Context, client *github.Client, owner, repo string) (*ActivitySummary, error) {
	var weeks []*github.WeeklyCommitActivity

	for attempt := 0; attempt <= activityStatsMaxRetries; attempt++ {
		result, _, err := client.Repositories.ListCommitActivity(ctx, owner, repo)
		if err == nil {
			weeks = result
			break
		}

		var accepted *github.AcceptedError
		if !errors.As(err, &accepted) {
			return nil, fmt.Errorf("fetching commit activity for %s/%s: %w", owner, repo, err)
		}

		if attempt == activityStatsMaxRetries {
			log.Printf("activity: %s/%s stats still not ready after %d retries, returning empty", owner, repo, activityStatsMaxRetries)
			return &ActivitySummary{}, nil
		}

		log.Printf("activity: %s/%s stats not ready (202), retrying in %s (attempt %d/%d)", owner, repo, activityStatsRetryWait, attempt+1, activityStatsMaxRetries)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(activityStatsRetryWait):
		}
	}

	last13 := weeks
	if len(weeks) > 13 {
		last13 = weeks[len(weeks)-13:]
	}

	total := 0
	activeWeeks := 0
	for _, w := range last13 {
		total += w.GetTotal()
		if w.GetTotal() > 0 {
			activeWeeks++
		}
	}

	return &ActivitySummary{
		TotalCommits90d:   total,
		ActiveWeeks90d:    activeWeeks,
		SuspiciousPadding: looksArtificiallyUniform(last13),
	}, nil
}

func looksArtificiallyUniform(weeks []*github.WeeklyCommitActivity) bool {
	if len(weeks) < 8 {
		return false
	}

	nonZero := make([]int, 0, len(weeks))
	for _, w := range weeks {
		if t := w.GetTotal(); t > 0 {
			nonZero = append(nonZero, t)
		}
	}
	if len(nonZero) < 8 {
		return false
	}

	first := nonZero[0]
	identical := true
	for _, v := range nonZero {
		if v != first {
			identical = false
			break
		}
	}
	return identical
}
