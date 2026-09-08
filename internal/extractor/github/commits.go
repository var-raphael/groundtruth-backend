package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v66/github"
)

type CommitTiming struct {
	Timestamp time.Time
	Message   string
}

func FetchRecentCommitTimings(ctx context.Context, client *github.Client, owner, repo string, limit int) ([]CommitTiming, error) {
	if limit <= 0 {
		limit = 50
	}

	opts := &github.CommitsListOptions{
		Author:      owner,
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
			continue
		}
		timings = append(timings, CommitTiming{
			Timestamp: author.GetDate().Time,
			Message:   c.GetCommit().GetMessage(),
		})
	}

	return timings, nil
}
