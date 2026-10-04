package worker

import (
	"context"
	"log"
	"net/http"

	"github.com/google/go-github/v66/github"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/models"
)

func pickGithubClient(ctx context.Context, serverClient *github.Client, c *models.Candidate) *github.Client {
	if c.GithubToken == nil || *c.GithubToken == "" {
		return serverClient
	}

	candidateClient := ghextractor.NewClient(*c.GithubToken)
	_, resp, err := candidateClient.Users.Get(ctx, "")
	if err != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized {
		log.Printf("scan: [github] stored token for %s was rejected, using the server token instead", c.GithubUsername)
		return serverClient
	}
	return candidateClient
}
