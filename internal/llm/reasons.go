package llm

import "strings"

const (
	maxPositiveReasons = 10
	maxNegativeReasons = 5
)

func EnforceReasonLimits(reasoning *JobReasoning) {
	if len(reasoning.PositiveReasons) > maxPositiveReasons {
		reasoning.PositiveReasons = reasoning.PositiveReasons[:maxPositiveReasons]
	}
	if len(reasoning.NegativeReasons) > maxNegativeReasons {
		reasoning.NegativeReasons = reasoning.NegativeReasons[:maxNegativeReasons]
	}

	for i := range reasoning.PositiveReasons {
		reasoning.PositiveReasons[i].Point = stripMarkdown(reasoning.PositiveReasons[i].Point)
	}
	for i := range reasoning.NegativeReasons {
		reasoning.NegativeReasons[i].Point = stripMarkdown(reasoning.NegativeReasons[i].Point)
	}
}

func stripMarkdown(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

func VerifyTrustFlagHonored(reasoning *JobReasoning, evidenceWarrantedFlag bool) (ok bool, warning string) {
	if evidenceWarrantedFlag && !reasoning.HasTrustFlag {
		return false, "evidence showed a trust concern (commit padding, junk directories, or a pushed .env file) but the model did not set has_trust_flag — this response should not be trusted as-is, consider re-running or flagging for manual review"
	}
	return true, ""
}

type RepoEvidenceSource struct {
	Name    string
	RepoURL string
	LiveURL string
}

func ResolveEvidence(reasoning *JobReasoning, repoSources []RepoEvidenceSource) (*ResolvedJobReasoning, []string) {
	byName := make(map[string]RepoEvidenceSource, len(repoSources))
	for _, r := range repoSources {
		byName[r.Name] = r
	}

	var unresolved []string

	resolve := func(reasons []Reason) []ResolvedReason {
		resolved := make([]ResolvedReason, 0, len(reasons))
		for _, r := range reasons {
			links := make([]EvidenceLink, 0, len(r.Evidence))
			for _, name := range r.Evidence {
				src, found := byName[name]
				if !found {
					unresolved = append(unresolved, name)
					links = append(links, EvidenceLink{Project: name})
					continue
				}
				links = append(links, EvidenceLink{
					Project: src.Name,
					RepoURL: src.RepoURL,
					LiveURL: src.LiveURL,
				})
			}
			resolved = append(resolved, ResolvedReason{Point: r.Point, Evidence: links})
		}
		return resolved
	}

	positives := resolve(reasoning.PositiveReasons)
	negatives := resolve(reasoning.NegativeReasons)

	return &ResolvedJobReasoning{
		Score:           reasoning.Score,
		StackMatch:      reasoning.StackMatch,
		PositiveReasons: positives,
		NegativeReasons: negatives,
		HasTrustFlag:    reasoning.HasTrustFlag,
	}, unresolved
}
