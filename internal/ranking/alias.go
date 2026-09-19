package ranking

import "strings"

var stackAliases = map[string]string{
	"postgres":   "postgresql",
	"postgresql": "postgresql",
	"supabase":   "postgresql",
	"js":         "javascript",
	"javascript": "javascript",
	"ts":         "typescript",
	"typescript": "typescript",
	"node":       "node.js",
	"nodejs":     "node.js",
	"node.js":    "node.js",
	"node.ts":    "node.js",
	"mongo":      "mongodb",
	"mongodb":    "mongodb",
}

func canonicalStackName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if canon, ok := stackAliases[n]; ok {
		return canon
	}
	return n
}

func StackNamesMatch(a, b string) bool {
	return canonicalStackName(a) == canonicalStackName(b)
}
