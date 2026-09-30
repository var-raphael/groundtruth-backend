package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v66/github"
)

type ReleaseInfo struct {
	Tag         string
	Name        string
	URL         string
	PublishedAt time.Time
}

func FetchLatestRelease(ctx context.Context, client *github.Client, owner, repo string) (*ReleaseInfo, error) {
	rel, resp, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("fetching latest release for %s/%s: %w", owner, repo, err)
	}
	if rel.GetHTMLURL() == "" || rel.GetTagName() == "" {
		return nil, nil
	}
	return &ReleaseInfo{
		Tag:         rel.GetTagName(),
		Name:        rel.GetName(),
		URL:         rel.GetHTMLURL(),
		PublishedAt: rel.GetPublishedAt().Time,
	}, nil
}
