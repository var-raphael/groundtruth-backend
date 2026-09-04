package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

// ForkStatus tells us whether a fork has real work on top of it, and
// what upstream it's compared against — needed for ranking to decide
// whether to treat this fork as one of the candidate's "own" repos.
type ForkStatus struct {
	IsFork          bool
	ParentOwner     string
	ParentRepo      string
	CommitsAhead    int // candidate's commits not present in upstream
}

// FetchForkStatus checks how many commits a fork is ahead of its parent.
// Only meaningful to call on repos where RawRepo.IsFork is true — the
// parent info comes from the repo's own metadata (GetRepo), and the ahead
// count comes from GitHub's compare API, one extra call per fork.
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

	return &ForkStatus{
		IsFork:       true,
		ParentOwner:  parentOwner,
		ParentRepo:   parentRepo,
		CommitsAhead: comparison.GetAheadBy(),
	}, nil
}
