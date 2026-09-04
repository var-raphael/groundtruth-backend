package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

// LanguageBreakdown is the real byte-count-per-language for a repo, not
// GitHub's single "primary language" guess. Fixes real false positives —
// e.g. a Python PDF-extraction tool with a bulkier HTML frontend reports
// as "HTML" via the single-language field despite Python being the actual
// functional core. This gives the real proportions so stack matching can
// judge "does Python appear meaningfully" instead of "is Python the winner."
type LanguageBreakdown map[string]int // language name -> bytes of code

// FetchLanguages pulls the real per-language byte breakdown for a repo.
// One cheap request, no pagination needed — GitHub returns the full map
// in a single response.
func FetchLanguages(ctx context.Context, client *github.Client, owner, repo string) (LanguageBreakdown, error) {
	langs, _, err := client.Repositories.ListLanguages(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("fetching languages for %s/%s: %w", owner, repo, err)
	}
	return LanguageBreakdown(langs), nil
}
