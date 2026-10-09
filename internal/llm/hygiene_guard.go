package llm

import "strings"

// envClaimMarkers are phrases that signal a negative reason is accusing a
// repo of pushing an environment file or leaking credentials.
var envClaimMarkers = []string{".env", "environment file", "credential"}

func claimsEnvLeak(point string) bool {
	p := strings.ToLower(point)
	for _, m := range envClaimMarkers {
		if strings.Contains(p, m) {
			return true
		}
	}
	return false
}

func citesRepoWithEnv(evidence []string, reposWithEnv map[string]bool) bool {
	for _, name := range evidence {
		if reposWithEnv[name] {
			return true
		}
	}
	return false
}

// DropUnsupportedEnvClaims removes negative reasons that accuse a repo of
// pushing an environment file or leaking credentials, unless at least one
// repo the reason cites really has an env file in its own extracted data.
// reposWithEnv holds the names of repos whose file tree contained one.
// It returns the text of every dropped reason so the caller can log it.
func DropUnsupportedEnvClaims(reasoning *JobReasoning, reposWithEnv map[string]bool) []string {
	kept := make([]Reason, 0, len(reasoning.NegativeReasons))
	var dropped []string
	for _, r := range reasoning.NegativeReasons {
		if !claimsEnvLeak(r.Point) || citesRepoWithEnv(r.Evidence, reposWithEnv) {
			kept = append(kept, r)
			continue
		}
		dropped = append(dropped, r.Point)
	}
	reasoning.NegativeReasons = kept
	return dropped
}
