package github

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/go-github/v66/github"
)

type RawContribution struct {
	RepoOwner        string
	RepoName         string
	RepoURL          string
	PRUrl            string
	PRTitle          string
	MergedAt         string
	MergedPRCount    int
	ContributorCount int
	Stars            int
}

func FetchExternalContributions(ctx context.Context, client *github.Client, username string) ([]RawContribution, error) {
	query := fmt.Sprintf("author:%s type:pr is:merged", username)
	opts := &github.SearchOptions{
		Sort:  "updated",
		Order: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	type prSummary struct {
		url      string
		title    string
		mergedAt string
	}

	mostRecentPRByRepo := make(map[string]prSummary)
	prCountByRepo := make(map[string]int)
	repoOrder := make([]string, 0)

	for {
		result, resp, err := client.Search.Issues(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("searching merged PRs for %s: %w", username, err)
		}

		for _, issue := range result.Issues {
			owner, repoName, ok := parseRepoFromIssueURL(issue.GetRepositoryURL())
			if !ok {
				continue
			}

			if owner == username {
				continue
			}

			key := owner + "/" + repoName
			if _, seen := prCountByRepo[key]; !seen {
				repoOrder = append(repoOrder, key)
			}
			prCountByRepo[key]++

			if _, exists := mostRecentPRByRepo[key]; !exists {
				mostRecentPRByRepo[key] = prSummary{
					url:      issue.GetHTMLURL(),
					title:    issue.GetTitle(),
					mergedAt: issue.GetClosedAt().String(),
				}
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	contributions := make([]RawContribution, 0, len(repoOrder))
	for _, key := range repoOrder {
		owner, repoName, ok := splitRepoKey(key)
		if !ok {
			continue
		}

		repo, _, err := client.Repositories.Get(ctx, owner, repoName)
		if err != nil {
			continue
		}

		contributorCount, err := fetchContributorCount(ctx, client, owner, repoName)
		if err != nil {
			contributorCount = 0
		}

		recent := mostRecentPRByRepo[key]

		contributions = append(contributions, RawContribution{
			RepoOwner:        owner,
			RepoName:         repoName,
			RepoURL:          repo.GetHTMLURL(),
			PRUrl:            recent.url,
			PRTitle:          recent.title,
			MergedAt:         recent.mergedAt,
			MergedPRCount:    prCountByRepo[key],
			ContributorCount: contributorCount,
			Stars:            repo.GetStargazersCount(),
		})
	}

	sort.Slice(contributions, func(i, j int) bool {
		return contributions[i].ContributorCount > contributions[j].ContributorCount
	})
	if len(contributions) > maxDistinctContributionRepos {
		contributions = contributions[:maxDistinctContributionRepos]
	}

	return contributions, nil
}

const maxDistinctContributionRepos = 5

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
	return len(contributors), nil
}

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

func splitRepoKey(key string) (owner, repo string, ok bool) {
	parts := splitOnce(key, '/')
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
