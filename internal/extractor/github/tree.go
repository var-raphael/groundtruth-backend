package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
)

// TreeSummary is the raw shape of a repo's file structure. Deliberately
// unopinionated — no "is this a real project" judgment happens here. Every
// path is handed over as-is, flat, and whatever mechanical rules or LLM
// judgment we build later decides what it means.
type TreeSummary struct {
	Paths []string // every file and directory path in the repo, flat

	HasReadme     bool
	ReadmeText    string // README content, truncated to readmeMaxChars if longer
	ReadmeTrunced bool   // true if the real README was longer than what's in ReadmeText
}

// readmeMaxChars caps how much README text ever reaches a prompt. Chosen
// as a character count (not word count) since token usage tracks characters
// much more closely than words — code blocks, tables, and URLs (all common
// in real READMEs, per Gnat/vexaro's actual docs) eat tokens in ways word
// count doesn't reflect. ~8000 chars is roughly 1500-2000 tokens for
// typical English/code mixed text — generous enough for real documentation,
// bounded enough that one unusually long README can't blow up a prompt.
const readmeMaxChars = 8000

// FetchTree pulls a repo's full file tree (paths only, no blob contents —
// cheap, one request) plus the README's actual text. No filtering, no
// derived booleans — evaluation of what this tree "means" happens later,
// either by the LLM or a separate mechanical pass, not here.
func FetchTree(ctx context.Context, client *github.Client, owner, repo, defaultBranch string) (*TreeSummary, error) {
	tree, _, err := client.Git.GetTree(ctx, owner, repo, defaultBranch, true)
	if err != nil {
		return nil, fmt.Errorf("fetching tree for %s/%s: %w", owner, repo, err)
	}

	summary := &TreeSummary{
		Paths: make([]string, 0, len(tree.Entries)),
	}
	for _, entry := range tree.Entries {
		summary.Paths = append(summary.Paths, entry.GetPath())
	}

	readme, _, err := client.Repositories.GetReadme(ctx, owner, repo, nil)
	if err != nil {
		// no README is a legitimate, common case — not a fetch failure
		summary.HasReadme = false
		return summary, nil
	}

	content, err := readme.GetContent()
	if err != nil {
		// README exists but couldn't be decoded — record that it exists,
		// leave the text empty rather than guessing at "thin" here
		summary.HasReadme = true
		return summary, nil
	}

	summary.HasReadme = true
	if len(content) > readmeMaxChars {
		summary.ReadmeText = content[:readmeMaxChars]
		summary.ReadmeTrunced = true
	} else {
		summary.ReadmeText = content
	}

	return summary, nil
}
