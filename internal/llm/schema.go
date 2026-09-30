package llm

import "encoding/json"

// EvidenceLink is one piece of resolved evidence backing a reason —
// matches the frontend's Evidence type (project/liveUrl/repoUrl). URLs
// here are NEVER taken from the model's own output — they're mechanically
// resolved from our own verified RawRepo data after the fact, so a
// recruiter-facing link is always guaranteed correct, never something the
// model could hallucinate or reformat wrong.
type EvidenceLink struct {
	Project    string `json:"project"`
	LiveURL    string `json:"liveUrl,omitempty"`
	ReleaseURL string `json:"releaseUrl,omitempty"`
	RepoURL    string `json:"repoUrl,omitempty"`
}

// Reason mirrors the JSON shape requested in the system prompt. Evidence
// here is what the MODEL returns — bare repo name strings only, per the
// system prompt's instructions. This is intentionally the raw/unresolved
// form; ResolvedReason (below) is the trustworthy version with real URLs
// attached, built mechanically after parsing, never from model output.
type Reason struct {
	Point    string   `json:"point"`
	Evidence []string `json:"evidence"` // bare repo names as the model returned them
}

// ResolvedReason is a Reason with its evidence upgraded from bare names to
// full EvidenceLinks with real, verified URLs — this is what should
// actually be persisted/rendered, never the raw Reason.
type ResolvedReason struct {
	Point    string         `json:"point"`
	Evidence []EvidenceLink `json:"evidence"`
}

// JobReasoning is the parsed result of one candidate scored against one
// job. Field names/tags match the exact JSON shape dictated in
// systemPrompt — if that shape ever changes, this struct and the prompt
// must be updated together, they are not independent.
type JobReasoning struct {
	Score           int      `json:"score"`
	StackMatch      string   `json:"stack_match"`
	PositiveReasons []Reason `json:"positive_reasons"`
	NegativeReasons []Reason `json:"negative_reasons"`
	HasTrustFlag    bool     `json:"has_trust_flag"`
}

// ResolvedJobReasoning is JobReasoning with every reason's evidence
// upgraded to real EvidenceLinks. This — not JobReasoning — is what
// should be persisted and sent to the frontend.
type ResolvedJobReasoning struct {
	Score           int              `json:"score"`
	StackMatch      string           `json:"stack_match"`
	PositiveReasons []ResolvedReason `json:"positive_reasons"`
	NegativeReasons []ResolvedReason `json:"negative_reasons"`
	HasTrustFlag    bool             `json:"has_trust_flag"`
}

// ParseJobReasoning parses the raw JSON string returned by CompleteJSON
// into a JobReasoning. Kept separate from the client so parsing/validation
// logic can evolve without touching the HTTP layer.
func ParseJobReasoning(raw string) (*JobReasoning, error) {
	var result JobReasoning
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DetectedStack is the parsed result of one repo's combined language-bytes
// + manifest-contents stack detection call. This is the single source of
// truth for what technologies a repo actually uses — it supersedes raw
// LanguageBreakdown for stack-matching purposes specifically, since it can
// see frameworks and databases that byte proportions alone cannot.
type DetectedStack struct {
	Stack []string `json:"stack"`
}

// ParseDetectedStack parses the raw JSON string returned by CompleteJSON
// for a stack-detection call into a DetectedStack.
func ParseDetectedStack(raw string) (*DetectedStack, error) {
	var result DetectedStack
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type DetectedStackBatchEntry struct {
	Name  string   `json:"name"`
	Stack []string `json:"stack"`
}

type DetectedStackBatch struct {
	Repos []DetectedStackBatchEntry `json:"repos"`
}

func ParseDetectedStackBatch(raw string) (*DetectedStackBatch, error) {
	var result DetectedStackBatch
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
