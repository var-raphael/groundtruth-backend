package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

type ForkStatus struct {
	IsFork       bool
	ParentOwner  string
	ParentRepo   string
	CommitsAhead int
	LinesChanged int
}

func FetchForkStatus(ctx context.Context, client *github.Client, owner, repo string) (*ForkStatus, error) {
	full, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("fetching repo metadata for %s/%s: %w", owner, repo, err)
	}

	if !full.GetFork() || full.GetParent() == nil {
		return &ForkStatus{IsFork: false}, nil
	}

	parentOwner := full.GetParent().GetOwner().GetLogin()
	parentRepo := full.GetParent().GetName()
	parentBranch := full.GetParent().GetDefaultBranch()
	candidateBranch := full.GetDefaultBranch()

	base := fmt.Sprintf("%s:%s", parentOwner, parentBranch)
	head := fmt.Sprintf("%s:%s", owner, candidateBranch)

	comparison, _, err := client.Repositories.CompareCommits(ctx, parentOwner, parentRepo, base, head, nil)
	if err != nil {
		return nil, fmt.Errorf("comparing %s/%s against parent: %w", owner, repo, err)
	}

	var linesChanged int
	for _, f := range comparison.Files {
		linesChanged += f.GetAdditions() + f.GetDeletions()
	}

	return &ForkStatus{
		IsFork:       true,
		ParentOwner:  parentOwner,
		ParentRepo:   parentRepo,
		CommitsAhead: comparison.GetAheadBy(),
		LinesChanged: linesChanged,
	}, nil
}
