package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

// RawRepo is the trimmed-down shape we actually care about from GitHub's
// repo list response, before any filtering/ranking happens.
type RawRepo struct {
	Name          string
	Description   string
	Language      string // GitHub's single "primary language" guess
	IsFork        bool
	PushedAt      string // last commit timestamp, ISO string for now
	HomepageURL   string // candidate-set "live URL" field, if any
	RepoURL       string // the actual github.com/owner/repo URL — this was missing before, needed to build real evidence links for the frontend
	StarCount     int
	Size          int    // KB — 0 usually means an empty repo
	DefaultBranch string // needed to fetch the tree (tree.go) against the right branch
}

// FetchOwnedRepos pulls every repo owned by username, sorted by most
// recently pushed. Empty repos are dropped here since there's nothing to
// evaluate; real fork-vs-original-work filtering happens in ranking/filter.go,
// not here — this stays a dumb fetch-and-shape step.
func FetchOwnedRepos(ctx context.Context, client *github.Client, username string) ([]RawRepo, error) {
	opts := &github.RepositoryListByUserOptions{
		Sort: "pushed",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var all []RawRepo
	for {
		repos, resp, err := client.Repositories.ListByUser(ctx, username, opts)
		if err != nil {
			return nil, fmt.Errorf("fetching repos for %s: %w", username, err)
		}

		for _, r := range repos {
			if r.GetSize() == 0 {
				continue // empty repo, nothing to evaluate
			}
			all = append(all, RawRepo{
				Name:          r.GetName(),
				Description:   r.GetDescription(),
				Language:      r.GetLanguage(),
				IsFork:        r.GetFork(),
				PushedAt:      r.GetPushedAt().String(),
				HomepageURL:   r.GetHomepage(),
				RepoURL:       r.GetHTMLURL(),
				StarCount:     r.GetStargazersCount(),
				Size:          r.GetSize(),
				DefaultBranch: r.GetDefaultBranch(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}
