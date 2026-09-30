package ranking

import (
	"context"
	"strings"

	"github.com/google/go-github/v66/github"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
)

const minCommitsAheadToCount = 3
const minLinesChangedToCount = 50

func FilterOwnedRepos(ctx context.Context, client *github.Client, username string, repos []ghextractor.RawRepo) ([]ghextractor.RawRepo, error) {
	var kept []ghextractor.RawRepo

	for _, r := range repos {
		if strings.EqualFold(r.Name, username) {
			continue
		}

		if !r.IsFork {
			kept = append(kept, r)
			continue
		}

		status, err := ghextractor.FetchForkStatus(ctx, client, username, r.Name)
		if err != nil {
			continue
		}

		if status.CommitsAhead >= minCommitsAheadToCount && status.LinesChanged >= minLinesChangedToCount {
			kept = append(kept, r)
		}
	}

	return kept, nil
}
