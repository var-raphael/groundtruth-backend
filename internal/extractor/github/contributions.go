package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

// RawContribution is one merged PR the candidate made on a repo they don't
// own. Flat and unopinionated, same philosophy as tree.go — we hand over
// the real numbers (contributors, stars) and let scoring/LLM judge weight,
// not a hardcoded threshold here.
type RawContribution struct {
	RepoOwner        string
	RepoName         string
	RepoURL          string
	PRUrl            string
	PRTitle          string
	MergedAt         string
	ContributorCount int
	Stars            int
}

// FetchExternalContributions finds every merged PR authored by username
// where the target repo is NOT owned by username — i.e. real external
// open-source contributions, not just their own commit history.
//
// Uses the Search API (issues/PRs), which is the only way to query "all
// merged PRs by this author" without walking every repo on GitHub. Search
// API has its own tighter rate limit (30 req/min authenticated, 10/min
// unauth) — worth knowing before running this against many candidates.
func FetchExternalContributions(ctx context.Context, client *github.Client, username string) ([]RawContribution, error) {
	query := fmt.Sprintf("author:%s type:pr is:merged", username)
	opts := &github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var contributions []RawContribution
	for {
		result, resp, err := client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("searching merged PRs for %s: %w", username, err)
		}

		for _, issue := range result.Issues {
			owner, repoName, ok := parseRepoFromIssueURL(issue.GetRepositoryURL())
			if !ok {
				continue // unexpected URL shape, skip rather than guess
			}

			if owner == username {
				continue // own repo — that's normal commit activity, not an external contribution
			}

			repo, _, err := client.Repositories.Get(ctx, owner, repoName)
			if err != nil {
				// target repo may have been deleted/renamed since the PR
				// merged — skip it rather than fail the whole fetch
				continue
			}

			contributorCount, err := fetchContributorCount(ctx, client, owner, repoName)
			if err != nil {
				contributorCount = 0 // unknown rather than fail — scoring can treat 0 as "uncertain"
			}

			contributions = append(contributions, RawContribution{
				RepoOwner:        owner,
				RepoName:         repoName,
				RepoURL:          repo.GetHTMLURL(),
				PRUrl:            issue.GetHTMLURL(),
				PRTitle:          issue.GetTitle(),
				MergedAt:         issue.GetClosedAt().String(),
				ContributorCount: contributorCount,
				Stars:            repo.GetStargazersCount(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return contributions, nil
}

// fetchContributorCount gets the total contributor count via the Link
// header's last page, rather than paginating through every contributor —
// one request instead of potentially hundreds for a large repo.
func fetchContributorCount(ctx context.Context, client *github.Client, owner, repo string) (int, error) {
	opts := &github.ListContributorsOptions{
		ListOptions: github.ListOptions{PerPage: 1},
	}

	contributors, resp, err := client.Repositories.ListContributors(ctx, owner, repo, opts)
	if err != nil {
		return 0, fmt.Errorf("listing contributors for %s/%s: %w", owner, repo, err)
	}

	if resp.LastPage > 0 {
		return resp.LastPage, nil
	}
	// no pagination happened — either 0 or 1 contributor total
	return len(contributors), nil
}

// parseRepoFromIssueURL extracts "owner", "repo" from a repository_url like
// "https://api.github.com/repos/owner/repo".
func parseRepoFromIssueURL(url string) (owner, repo string, ok bool) {
	const marker = "/repos/"
	idx := indexOf(url, marker)
	if idx == -1 {
		return "", "", false
	}
	rest := url[idx+len(marker):]
	parts := splitOnce(rest, '/')
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func splitOnce(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
