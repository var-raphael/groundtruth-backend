// Standalone debug tool. Fetches ONE repo directly, skipping
// FilterOwnedRepos/BuildTopRepos entirely, and prints exactly what
// BuildUserPrompt would send the LLM for it, plus what the mechanical
// trust-flag enforcement would do. Useful for validating prompt/flag
// behavior against a repo that wouldn't naturally survive top-K ranking.
//
// Usage:
//
//	go run ./cmd/promptdebug <owner> <repo> [githubToken]
package main

import (
	"context"
	"fmt"
	"os"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/ranking"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run ./cmd/promptdebug <owner> <repo> [githubToken]")
		os.Exit(1)
	}
	owner := os.Args[1]
	repoName := os.Args[2]
	token := ""
	if len(os.Args) > 3 {
		token = os.Args[3]
	}
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}

	client := ghextractor.NewClient(token)
	ctx := context.Background()

	repos, err := ghextractor.FetchOwnedRepos(ctx, client, owner)
	if err != nil {
		fatalf("fetching repos for %s: %v", owner, err)
	}
	var raw *ghextractor.RawRepo
	for i := range repos {
		if repos[i].Name == repoName {
			raw = &repos[i]
			break
		}
	}
	if raw == nil {
		fatalf("repo %s not found under %s (or it's empty/size=0, which FetchOwnedRepos skips)", repoName, owner)
	}

	tree, err := ghextractor.FetchTree(ctx, client, owner, repoName, raw.DefaultBranch)
	if err != nil {
		fatalf("fetching tree: %v", err)
	}

	languages, err := ghextractor.FetchLanguages(ctx, client, owner, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: fetching languages: %v\n", err)
	}

	jobStack := []string{"JavaScript", "TypeScript", "React"}

	sr := ranking.ScoreRepo(*raw, nil, tree, languages, jobStack)

	fmt.Println("=== JunkSignals detected ===")
	fmt.Printf("JunkDirs: %v\n", tree.Junk.JunkDirs)
	fmt.Printf("EnvFiles: %v\n", tree.Junk.EnvFiles)
	fmt.Printf("HasIssue: %v\n\n", tree.Junk.HasIssue())

	job := llm.JobContext{
		Title:       "Frontend Engineer",
		Description: "Building our core product UI.",
		Stack:       jobStack,
	}

	prompt := llm.BuildUserPrompt(job, []ranking.ScoredRepo{sr}, nil)
	fmt.Println("=== Rendered user prompt ===")
	fmt.Println(prompt)

	fmt.Println("=== Trust-flag enforcement dry run ===")
	fakeReasoning := &llm.JobReasoning{Score: 5, StackMatch: "partial", HasTrustFlag: false}
	ok, warning := llm.VerifyTrustFlagHonored(fakeReasoning, tree.Junk.HasIssue())
	fmt.Printf("If the model returned has_trust_flag=false: honored=%v\n", ok)
	if warning != "" {
		fmt.Printf("Warning that would surface: %s\n", warning)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
