package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v66/github"
)

// CommitTiming is one commit's real timestamp — raw enough for the LLM
// stage to reason about patterns itself ("mostly evenings and weekends"
// vs "steady weekday 9-5"), rather than us pre-deciding what a "founding
// engineer pattern" looks like in Go code.
type CommitTiming struct {
	Timestamp time.Time
	Message   string // short, gives the LLM some context on what was being worked on
}

// FetchRecentCommitTimings pulls real commit timestamps for a repo, most
// recent first, capped at `limit`. This is the raw material for
// seniority/role-fit reasoning (title says "Founding Engineer" — do
// commits show odd-hours, weekend intensity? title says "Senior" — is
// there a steadier, longer-horizon pattern?) — the LLM does that judgment,
// this just hands over real timestamps, no interpretation here.
func FetchRecentCommitTimings(ctx context.Context, client *github.Client, owner, repo string, limit int) ([]CommitTiming, error) {
	if limit <= 0 {
		limit = 50
	}

	opts := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: limit},
	}

	commits, _, err := client.Repositories.ListCommits(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("fetching commits for %s/%s: %w", owner, repo, err)
	}

	timings := make([]CommitTiming, 0, len(commits))
	for _, c := range commits {
		author := c.GetCommit().GetAuthor()
		if author == nil || author.GetDate().IsZero() {
			continue // some commits lack author date info — skip rather than guess
		}
		timings = append(timings, CommitTiming{
			Timestamp: author.GetDate().Time,
			Message:   c.GetCommit().GetMessage(),
		})
	}

	return timings, nil
}
