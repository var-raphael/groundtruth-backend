package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/git-pkgs/manifests"
	"github.com/google/go-github/v66/github"
)

type ManifestFile struct {
	Path      string
	Ecosystem string
	Content   string
}

const maxManifestFiles = 10
const maxManifestBytes = 20000

func FindManifestPaths(paths []string) []string {
	var found []string
	for _, p := range paths {
		if _, _, ok := manifests.Identify(p); ok {
			found = append(found, p)
		}
	}
	if len(found) > maxManifestFiles {
		found = found[:maxManifestFiles]
	}
	return found
}

func FetchManifestFiles(ctx context.Context, client *github.Client, owner, repo string, manifestPaths []string) ([]ManifestFile, error) {
	if len(manifestPaths) == 0 {
		return nil, nil
	}

	var files []ManifestFile
	for _, p := range manifestPaths {
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
		if len(content) > maxManifestBytes {
			content = content[:maxManifestBytes]
		}

		ecosystem := ""
		if parsed, parseErr := manifests.Parse(p, []byte(content)); parseErr == nil {
			ecosystem = parsed.Ecosystem
		}

		files = append(files, ManifestFile{Path: p, Ecosystem: ecosystem, Content: content})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("fetching manifest files for %s/%s: no manifests could be read", owner, repo)
	}
	return files, nil
}

func FormatManifestFiles(files []ManifestFile) string {
	var b strings.Builder
	for _, f := range files {
		if f.Ecosystem != "" {
			fmt.Fprintf(&b, "--- %s (%s) ---\n%s\n\n", f.Path, f.Ecosystem, f.Content)
		} else {
			fmt.Fprintf(&b, "--- %s ---\n%s\n\n", f.Path, f.Content)
		}
	}
	return b.String()
}
