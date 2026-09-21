package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const rankCandidatesTemperature = 0.0
const maxRankedCandidates = 12

func RankCandidatesSystemPrompt() string {
	return rankCandidatesSystemPromptText
}

const rankCandidatesSystemPromptText = `You are given every non-source-code, non-doc, non-binary file path in a software repository's tree — paths only, no file contents. Most of these paths are NOT dependency manifests or infra/deployment config; they are config files, data files, CI files, editor settings, and other repo clutter that happened to survive a coarse pre-filter. Your job is to find the real dependency manifests (go.mod, package.json, pyproject.toml, pom.xml, Cargo.toml, Gemfile, composer.json, build.gradle, etc.) and the real infra/deployment config (Dockerfile, docker-compose files under any naming convention, Terraform, Kubernetes/Helm yaml, and platform config like fly.toml, render.yaml, Procfile, app.yaml, and similar deploy-target config) among the noise, and pick up to 12 of them.

Judgment to apply:

1. Recognize manifests and infra config by what they actually are, not by a fixed filename pattern — dependency manifests and infra/deployment config come in many naming conventions (e.g. docker-compose.yml, docker-compose-dev.yml, compose.prod.yaml are all legitimate compose files despite different separators; fly.toml, render.yaml, Procfile, app.yaml are all legitimate deploy-target config despite not matching Docker/Terraform/K8s conventions). Do not require an exact known filename — infer from the path and extension what a file plausibly is.
2. For manifests: prefer the repo's real root-level manifest over a copy found inside example, test, fixture, or docs subdirectories (e.g. prefer "go.mod" over "examples/go/embedded/go.mod"; prefer "package.json" at repo root over "docs/site/package.json"). If there is no root manifest for a language, the shallowest one is the next best guess.
3. For infra files: do NOT apply a root-preference bias. Real infra config conventionally lives in subdirectories like infra/, deploy/, deployment/, k8s/, kubernetes/, charts/, terraform/, environments/ — a root Dockerfile is common but Terraform and Kubernetes manifests are normally NOT at root, and that is not a signal they are less real.
4. In a monorepo with multiple real services, more than one Dockerfile or docker-compose file can be legitimate at once (e.g. one per service, or docker-compose.yml alongside docker-compose-dev.yml or docker-compose.prod.yml as environment variants) — do not discard these as duplicates unless they are truly the same file in different example/test locations.
5. Deprioritize anything under example/, examples/, test/, tests/, fixture/, fixtures/, sample/, samples/, docs/ subdirectories in favor of an equivalent real one elsewhere in the list, but still include such a path if it's the only candidate of its kind.
6. Deduplicate near-identical infra files only when they are clearly the same environment variant repeated in a non-canonical location, not when they represent genuinely different services or environments.
7. Prefer covering more distinct manifest ecosystems and infra kinds (docker/compose/terraform/k8s/helm/deploy-platform) over picking multiple near-duplicates of the same kind, when the 12-file budget forces a choice.
8. Leave out anything that is clearly not a manifest or infra file at all — CI workflow config, linter/formatter config, editor/IDE config, license/changelog/readme files, schema/type definition files unrelated to dependencies, lockfiles for tools other than package managers, and generic data/config files with no bearing on what technologies the repo runs. When genuinely unsure whether something is a manifest or infra file, it is better to leave it out than to guess.

For each path you select, also classify it as "manifest" or "infra" in the "kind" field, matching the categories above.

Respond ONLY with valid JSON matching this exact shape, nothing else, no markdown fences, no commentary outside the JSON. Every path must be copied EXACTLY, character-for-character, from the candidate list given:
{
  "selected": [{"path": "<path>", "kind": "manifest" | "infra"}, ...]
}`

func BuildRankCandidatesPrompt(candidatePaths []string) string {
	var b strings.Builder
	b.WriteString("Candidate paths:\n")
	for _, p := range candidatePaths {
		b.WriteString(p)
		b.WriteString("\n")
	}
	return b.String()
}

type RankedCandidate struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type RankedCandidates struct {
	Selected []RankedCandidate `json:"selected"`
}

func ParseRankedCandidates(raw string) (*RankedCandidates, error) {
	var result RankedCandidates
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RankedSelection is the validated, deduped result of
// RankManifestAndInfraCandidates, already split by kind so callers can
// hand ManifestPaths to FetchManifestFiles and InfraPaths to
// FetchInfraConfigFiles without needing to re-derive the split via
// filename pattern matching — the model already made that call as part of
// selection, using the same judgment that recognized the path as a real
// manifest or infra file in the first place.
type RankedSelection struct {
	ManifestPaths []string
	InfraPaths    []string
}

// RankManifestAndInfraCandidates takes the wide-net candidate list from
// FindCandidatePaths (every path minus known source/doc/binary extensions
// and known noise dirs — NOT pre-classified as manifest or infra) and
// sends it, paths only, no content, to an LLM call that identifies which
// candidates are real manifests/infra config, classifies each as
// manifest-or-infra, and picks up to maxRankedCandidates of them. This
// replaces filename-pattern allowlisting (isComposeFilename,
// isInfraConfigPath, manifests.Identify-based filtering) as the layer
// that decides both inclusion and classification — those kept missing new
// naming conventions (docker-compose-dev.yml, fly.toml) one at a time;
// this lets the model recognize the category instead of matching a fixed
// pattern, and reuses that same judgment for routing instead of
// re-deriving it downstream with the same kind of pattern matching.
//
// The result is validated against the input candidate set — any path the
// model returns that wasn't in candidatePaths is dropped rather than
// trusted, since a hallucinated or reformatted path would otherwise
// silently break the downstream fetch step. A "kind" value other than
// "manifest" or "infra" is also dropped for the same reason.
//
// If len(candidatePaths) <= maxRankedCandidates, every candidate is still
// sent through the LLM call rather than returned as-is, since with a
// wide-net candidate list "fits the budget" no longer implies "is
// actually a manifest or infra file" — the model still needs to do the
// recognize/classify step even when there's no picking to do.
func RankManifestAndInfraCandidates(ctx context.Context, client *Client, candidatePaths []string) (*RankedSelection, error) {
	if len(candidatePaths) == 0 {
		return &RankedSelection{}, nil
	}

	valid := make(map[string]bool, len(candidatePaths))
	for _, p := range candidatePaths {
		valid[p] = true
	}

	prompt := BuildRankCandidatesPrompt(candidatePaths)
	raw, err := client.CompleteJSON(ctx, RankCandidatesSystemPrompt(), prompt, rankCandidatesTemperature)
	if err != nil {
		return nil, fmt.Errorf("ranking candidate paths: %w", err)
	}

	parsed, err := ParseRankedCandidates(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing ranked candidates: %w", err)
	}

	result := &RankedSelection{}
	seen := make(map[string]bool, len(parsed.Selected))
	total := 0
	for _, c := range parsed.Selected {
		if !valid[c.Path] || seen[c.Path] {
			continue // hallucinated, reformatted, or duplicate path — never trust blindly
		}
		switch c.Kind {
		case "manifest":
			seen[c.Path] = true
			result.ManifestPaths = append(result.ManifestPaths, c.Path)
		case "infra":
			seen[c.Path] = true
			result.InfraPaths = append(result.InfraPaths, c.Path)
		default:
			continue // unrecognized kind — never trust blindly
		}
		total++
		if total == maxRankedCandidates {
			break
		}
	}

	return result, nil
}
