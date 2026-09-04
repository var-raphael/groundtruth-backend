package outreach

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/models"
)

type Draft struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

const draftTemperature = 0.4

func BuildDraft(ctx context.Context, client *llm.Client, report *models.CandidateReport, job models.Job) (*Draft, error) {
	if len(report.Reasoning.PositiveReasons) == 0 {
		return nil, fmt.Errorf("no positive reasons available to draft outreach for candidate %s", report.CandidateID)
	}

	systemPrompt := systemPrompt()
	userPrompt := buildUserPrompt(report, job)

	raw, err := client.CompleteJSON(ctx, systemPrompt, userPrompt, draftTemperature)
	if err != nil {
		return nil, fmt.Errorf("drafting outreach for candidate %s: %w", report.CandidateID, err)
	}

	var draft Draft
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		return nil, fmt.Errorf("parsing outreach draft for candidate %s: %w (raw: %s)", report.CandidateID, err, truncate(raw, 500))
	}

	if strings.TrimSpace(draft.Subject) == "" || strings.TrimSpace(draft.Body) == "" {
		return nil, fmt.Errorf("outreach draft for candidate %s came back with empty subject or body", report.CandidateID)
	}

	return &draft, nil
}

func systemPrompt() string {
	return `You write short, personal outreach emails from a founder to a candidate whose GitHub work was just reviewed for a specific role. You will be given the candidate's name, the job they're being considered for, and a list of specific, verified positive things about their work (with evidence links).

Rules:
- Sound like a real founder personally reached out after actually reading the candidate's work — not like a recruiter template, not like a mail merge.
- Reference ONE OR TWO of the most specific, concrete details provided (a real project name, what it does, a real technical detail). Do not vaguely summarize all of them.
- Never invent facts, numbers, or details not given to you. Only use what's in the positive reasons and evidence provided.
- Keep it short: 3-5 sentences in the body. No filler, no corporate language, no exclamation-point enthusiasm.
- End with a soft, low-pressure call to action (e.g. interest in chatting), not a hard pitch or a demand.
- Sign off with "Best," and nothing after it — no name, the sender will add their own.
- Open with "Hi [first name]," as the greeting — always include "Hi", never open with just the name alone.
- Do not mention the candidate's score, ranking, or that they were evaluated/scored by anything. Write as if you personally noticed their work.
- Never use em dashes (—) or en dashes (–) anywhere in the email. Use a period, comma, or parentheses instead.
- Return ONLY valid JSON matching this exact shape, nothing else, no markdown fences:
{"subject": "...", "body": "..."}`
}

func buildUserPrompt(report *models.CandidateReport, job models.Job) string {
	var b strings.Builder

	firstName := strings.SplitN(report.Candidate.Name, " ", 2)[0]

	fmt.Fprintf(&b, "Candidate: %s\n", report.Candidate.Name)
	fmt.Fprintf(&b, "First name (use this in the greeting): %s\n\n", firstName)

	fmt.Fprintf(&b, "Job title: %s\n", job.Title)
	if strings.TrimSpace(job.Description) != "" {
		fmt.Fprintf(&b, "Job description: %s\n", job.Description)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Verified positive things about this candidate's work (pick 1-2 of the most specific/compelling to reference):")
	for i, reason := range report.Reasoning.PositiveReasons {
		fmt.Fprintf(&b, "%d. %s\n", i+1, reason.Point)
		for _, ev := range reason.Evidence {
			fmt.Fprintf(&b, "   - project: %s", ev.Project)
			if ev.LiveURL != "" {
				fmt.Fprintf(&b, " (live: %s)", ev.LiveURL)
			}
			fmt.Fprintln(&b)
		}
	}

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
