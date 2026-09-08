package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/scoring"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env found")
	}
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		log.Fatal("MISTRAL_API_KEY not set")
	}

	ctx := context.Background()
	githubToken := os.Getenv("GITHUB_TOKEN")
	githubClient := ghextractor.NewClient(githubToken)
	mistralClient := llm.NewClient(apiKey)
	username := "var-raphael"

	job := llm.JobContext{
		Title:       "Founding Full-Stack (AI)",
		Description: "We're building the verification layer for technical hiring. You'll own the pipeline end to end: ingest, scoring, and the report itself.",
		Stack:       []string{"Go", "TypeScript", "Python"},
	}

	f, err := os.Create("test.txt")
	if err != nil {
		log.Fatalf("could not create test.txt: %v", err)
	}
	defer f.Close()

	result, err := scoring.ScoreCandidate(ctx, githubClient, mistralClient, username, job)
	if err != nil {
		fmt.Fprintf(f, "PIPELINE ERROR: %v\n", err)
		fmt.Println("Done (with error).")
		return
	}

	if result.ContributionsError != "" {
		fmt.Fprintf(f, "CONTRIBUTIONS ERROR: %s\n", result.ContributionsError)
	}
	fmt.Fprintf(f, "Contributions found: %d distinct repos\n", len(result.Contributions))
	for _, c := range result.Contributions {
		fmt.Fprintf(f, "- %s/%s (%d contributors, %d merged PRs by candidate)\n", c.RepoOwner, c.RepoName, c.ContributorCount, c.MergedPRCount)
	}
	fmt.Fprintln(f)

	info := scoring.CandidateInfo{
		ID:              "cand-test-001",
		Name:            "Raphael Samuel",
		GithubUsername:  username,
		Email:           "raphael@var-raphael.dev",
		Country:         "Nigeria",
		YearsExperience: 6,
	}

	report := scoring.BuildReport("cand-test-001", "founding-fullstack-ai-001", info, job.Stack, result)

	fmt.Fprintf(f, "Stack Match:       %.1f/10\n", report.Reasoning.Breakdown.StackMatch)
	fmt.Fprintf(f, "Evidence Strength: %.1f/10\n", report.Reasoning.Breakdown.EvidenceStrength)
	fmt.Fprintf(f, "Contributions:     %.1f/10\n", report.Reasoning.Breakdown.Contributions)
	fmt.Fprintf(f, "LLM Judgment:      %.1f/10\n", report.Reasoning.Breakdown.LLMJudgment)
	fmt.Fprintf(f, "FINAL SCORE:       %.1f/10\n\n", report.Reasoning.Score)

	pretty, _ := json.MarshalIndent(report.Reasoning, "", "  ")
	fmt.Fprintln(f, string(pretty))

	if err := f.Sync(); err != nil {
		fmt.Println("f.Sync error:", err)
	}

	fmt.Println("Done. Results written to test.txt")
}
