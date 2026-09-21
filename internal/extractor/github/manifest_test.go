package github

import (
	"reflect"
	"testing"
)

func TestFindManifestPaths(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  []string
	}{
		{
			name:  "real manifests found",
			paths: []string{"go.mod", "package.json", "src/main.go"},
			want:  []string{"go.mod", "package.json"},
		},
		{
			name: "github actions workflows excluded",
			paths: []string{
				".github/workflows/ci.yml",
				".github/workflows/release.yml",
				"go.mod",
			},
			want: []string{"go.mod"},
		},
		{
			name:  "gitlab ci excluded",
			paths: []string{".gitlab-ci.yml", "package.json"},
			want:  []string{"package.json"},
		},
		{
			name:  "circleci excluded",
			paths: []string{".circleci/config.yml", "requirements.txt"},
			want:  []string{"requirements.txt"},
		},
		{
			name:  "jenkinsfile excluded",
			paths: []string{"Jenkinsfile", "composer.json"},
			want:  []string{"composer.json"},
		},
		{
			name: "cap at 5 real manifests, ci noise never competes for slots",
			paths: []string{
				".github/workflows/a.yml", ".github/workflows/b.yml", ".github/workflows/c.yml",
				".github/workflows/d.yml", ".github/workflows/e.yml", ".github/workflows/f.yml",
				"go.mod", "sdks/python/pyproject.toml", "sdks/typescript/package.json",
				"frontend/app/package.json", "cmd/hatchet-cli/go.mod", "hack/dev/compression-test/go.mod",
			},
			want: []string{
				"go.mod", "sdks/python/pyproject.toml", "sdks/typescript/package.json",
				"frontend/app/package.json", "cmd/hatchet-cli/go.mod",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindManifestPaths(tc.paths)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindManifestPaths(%v) = %v, want %v", tc.paths, got, tc.want)
			}
		})
	}
}
