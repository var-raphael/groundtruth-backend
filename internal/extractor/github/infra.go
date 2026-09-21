package github

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/google/go-github/v66/github"
)

type InfraConfigFile struct {
	Path    string
	Kind    string
	Content string
}

const maxCombinedManifestAndInfraFiles = 12
const maxInfraConfigBytes = 20000
const minInfraContentBytes = 5

func isComposeFilename(base string) bool {
	lower := strings.ToLower(base)
	if !strings.HasSuffix(lower, ".yml") && !strings.HasSuffix(lower, ".yaml") {
		return false
	}
	trimmed := strings.TrimSuffix(strings.TrimSuffix(lower, ".yml"), ".yaml")
	return trimmed == "docker-compose" ||
		trimmed == "compose" ||
		strings.HasPrefix(trimmed, "docker-compose.") ||
		strings.HasPrefix(trimmed, "compose.")
}

var k8sDirNames = map[string]bool{
	"k8s":         true,
	"kubernetes":  true,
	"deploy":      true,
	"deployment":  true,
	"deployments": true,
	"manifests":   true,
	"kube":        true,
	".k8s":        true,
}

func FindInfraConfigPaths(paths []string, manifestSlotsUsed int) []string {
	budget := maxCombinedManifestAndInfraFiles - manifestSlotsUsed
	if budget <= 0 {
		return nil
	}

	var found []string
	for _, p := range paths {
		if isInfraConfigPath(p) {
			found = append(found, p)
		}
	}
	if len(found) > budget {
		found = found[:budget]
	}
	return found
}

func FindAllInfraConfigPaths(paths []string) []string {
	var found []string
	for _, p := range paths {
		if isInfraConfigPath(p) {
			found = append(found, p)
		}
	}
	return found
}

func isInfraConfigPath(p string) bool {
	base := path.Base(p)

	if base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") {
		return true
	}

	if isComposeFilename(base) {
		return true
	}

	if strings.HasSuffix(base, ".tf") {
		return true
	}

	if base == "Chart.yaml" {
		return true
	}

	if base == "" || (!strings.HasSuffix(base, ".yaml") && !strings.HasSuffix(base, ".yml")) {
		return false
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if k8sDirNames[seg] {
			return true
		}
	}
	return false
}

func FetchInfraConfigFiles(ctx context.Context, client *github.Client, owner, repo string, infraPaths []string) ([]InfraConfigFile, error) {
	if len(infraPaths) == 0 {
		return nil, nil
	}

	var files []InfraConfigFile
	for _, p := range infraPaths {
		fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, p, nil)
		if err != nil {
			continue
		}
		if fileContent == nil {
			continue
		}
		content, err := fileContent.GetContent()
		if err != nil {
			continue
		}
		if len(strings.TrimSpace(content)) < minInfraContentBytes {
			continue
		}
		if len(content) > maxInfraConfigBytes {
			content = content[:maxInfraConfigBytes]
		}

		files = append(files, InfraConfigFile{Path: p, Kind: infraConfigKind(p), Content: content})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("fetching infra config files for %s/%s: no files could be read", owner, repo)
	}
	return files, nil
}

func infraConfigKind(p string) string {
	base := path.Base(p)
	switch {
	case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
		return "docker"
	case isComposeFilename(base):
		return "compose"
	case strings.HasSuffix(base, ".tf"):
		return "terraform"
	case base == "Chart.yaml":
		return "helm"
	default:
		return "kubernetes"
	}
}

func FormatInfraConfigFiles(files []InfraConfigFile) string {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "--- %s (%s) ---\n%s\n\n", f.Path, f.Kind, f.Content)
	}
	return b.String()
}
