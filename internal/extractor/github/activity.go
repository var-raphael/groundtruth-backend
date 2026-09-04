package github

import (
	"context"
	"errors"
	"fmt"

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

// FetchActivity pulls the 52-week commit activity for a repo and reduces it
// to the last ~13 weeks (roughly 90 days) for recency/frequency scoring.
//
// Note: GitHub computes these stats async on first request for a repo that
// hasn't been queried recently — a 202 response means "come back shortly,"
// not an error. Caller should treat a 202 as "no data yet" rather than fail.
func FetchActivity(ctx context.Context, client *github.Client, owner, repo string) (*ActivitySummary, error) {
	weeks, _, err := client.Repositories.ListCommitActivity(ctx, owner, repo)
	if err != nil {
		// go-github surfaces GitHub's 202 ("stats still being computed on
		// their side") as an *AcceptedError, not as a normal HTTP status we
		// can inspect on the response — check for it explicitly rather than
		// treating it as a real failure. Worker should retry this repo later.
		var accepted *github.AcceptedError
		if errors.As(err, &accepted) {
			return &ActivitySummary{}, nil
		}
		return nil, fmt.Errorf("fetching commit activity for %s/%s: %w", owner, repo, err)
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
