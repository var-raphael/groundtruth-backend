package ranking

import (
	"context"
	"strings"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/google/go-github/v66/github"
)

// minCommitsAheadToCount is the bar for "this fork has real work on it,
// not just a stray commit or a merge-conflict resolution." Deliberately
// low and simple — we're not judging quality here, just "is there
// something to actually evaluate."
const minCommitsAheadToCount = 1

// FilterOwnedRepos decides which of a candidate's repos are worth ranking:
//   - the special GitHub profile-README repo (repo name == username) is
//     always excluded — GitHub reserves this specific repo name to render
//     as a personal bio page on the user's profile, not a software
//     project. Its README is self-description prose ("I'm a full-stack
//     engineer with 6 years..."), structurally identical in tone to a
//     resume — scoring it as project evidence let an LLM cite bio claims
//     ("I've written Python for 6 years") as if they were verified
//     language/repo evidence, which is exactly the kind of unverified
//     claim this product exists to NOT trust.
//   - non-forks: always kept
//   - forks: kept ONLY if the candidate has real commits ahead of the
//     upstream repo — i.e. they didn't just bookmark someone else's work,
//     they actually built something on top of it. Once kept, it's treated
//     exactly like any other owned repo from here on.
//   - forks with zero commits ahead: dropped, nothing of the candidate's
//     own to evaluate here (their real external work, if any, is already
//     captured separately by contributions.go via merged PRs)
//
// Deliberately not filtering on recency, stack match, or repo size here —
// those are ranking signals (score.go), not exclusion rules.
func FilterOwnedRepos(ctx context.Context, client *github.Client, username string, repos []ghextractor.RawRepo) ([]ghextractor.RawRepo, error) {
	var kept []ghextractor.RawRepo

	for _, r := range repos {
		if strings.EqualFold(r.Name, username) {
			continue // profile-README repo, not a project — see comment above
		}

		if !r.IsFork {
			kept = append(kept, r)
			continue
		}

		status, err := ghextractor.FetchForkStatus(ctx, client, username, r.Name)
		if err != nil {
			// couldn't determine fork status — safer to drop than to
			// guess, since an unverified fork isn't evidence of anything
			continue
		}

		if status.CommitsAhead >= minCommitsAheadToCount {
			kept = append(kept, r)
		}
	}

	return kept, nil
}
