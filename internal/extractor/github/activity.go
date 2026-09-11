package github

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/go-github/v66/github"
)

// WeeklyCommits mirrors GitHub's stats/commit_activity response: one entry
// per week over the last 52 weeks, with a total commit count for that week.
type WeeklyCommits struct {
	WeekStart int64 // unix timestamp, start of week
	Total     int
}

// ActivitySummary is the shaped-down view ranking/score.go actually needs.
type ActivitySummary struct {
	TotalCommits90d int
	ActiveWeeks90d  int // how many of the last ~13 weeks had at least 1 commit

	// authenticity signal: true if the weekly pattern looks artificially
	// uniform (near-identical commit counts every single week, no natural
	// variance) rather than the bursty, uneven pattern real work produces.
	SuspiciousPadding bool
}

const (
	activityStatsMaxRetries = 3
	activityStatsRetryWait  = 2 * time.Second
)

// FetchActivity pulls the 52-week commit activity for a repo and reduces it
// to the last ~13 weeks (roughly 90 days) for recency/frequency scoring.
//
// GitHub computes these stats asynchronously on first request for a repo
// that hasn't been queried recently — a 202 response means "still
// computing, try again shortly," not "zero activity." Returning zeros for
// a 202 would silently misrepresent a candidate's real activity as
// nonexistent, so this retries a few times with a short wait before giving
// up and returning empty data.
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

// looksArtificiallyUniform is a deliberately simple, honest heuristic — not
// a confident fraud verdict. Real human activity has natural variance week
// to week (bursts around real problems, quiet weeks around other work).
// A long run of weeks with near-identical, nonzero commit counts is unusual
// enough to flag for a closer look, not enough on its own to penalize hard.
func looksArtificiallyUniform(weeks []*github.WeeklyCommitActivity) bool {
	if len(weeks) < 8 {
		return false // not enough data to say anything meaningful
	}

	nonZero := make([]int, 0, len(weeks))
	for _, w := range weeks {
		if t := w.GetTotal(); t > 0 {
			nonZero = append(nonZero, t)
		}
	}
	if len(nonZero) < 8 {
		return false // too sparse to judge uniformity
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
