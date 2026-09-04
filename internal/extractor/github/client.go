package github

import (
	"context"

	"github.com/google/go-github/v66/github"
)

// NewClient returns a GitHub API client. Pass an empty token for
// unauthenticated access (60 req/hr, fine for building/testing against one
// profile). Pass a real candidate OAuth token later — same call site, no
// other code changes needed, since go-github handles both the same way.
func NewClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}
	return github.NewClient(nil).WithAuthToken(token)
}

// ctx is threaded through everywhere so timeouts/cancellation work once
// this runs inside the worker pipeline instead of a quick manual test.
func BackgroundCtx() context.Context {
	return context.Background()
}
