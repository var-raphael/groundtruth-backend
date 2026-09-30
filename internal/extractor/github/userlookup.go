package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

func FetchUserID(ctx context.Context, client *github.Client, username string) (int64, error) {
	user, _, err := client.Users.Get(ctx, username)
	if err != nil {
		return 0, fmt.Errorf("fetching github user %s: %w", username, err)
	}
	if user.ID == nil {
		return 0, fmt.Errorf("github user %s has no numeric id", username)
	}
	return *user.ID, nil
}
