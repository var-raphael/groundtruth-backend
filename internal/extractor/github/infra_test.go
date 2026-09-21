package github

import (
	"reflect"
	"testing"
)

func TestFindInfraConfigPaths(t *testing.T) {
	cases := []struct {
		name              string
		paths             []string
		manifestSlotsUsed int
		want              []string
	}{
		{
			name:  "dockerfile at root",
			paths: []string{"Dockerfile", "src/main.go"},
			want:  []string{"Dockerfile"},
		},
		{
			name:  "dockerfile variants",
			paths: []string{"Dockerfile.dev", "Dockerfile.prod", "docker/Dockerfile", "README.md"},
			want:  []string{"Dockerfile.dev", "Dockerfile.prod", "docker/Dockerfile"},
		},
		{
			name:  "compose variants",
			paths: []string{"docker-compose.yml", "docker-compose.prod.yaml", "compose.yaml", "nginx.conf"},
			want:  []string{"docker-compose.yml", "docker-compose.prod.yaml", "compose.yaml"},
		},
		{
			name:  "compose variants with arbitrary env suffixes (hatchet-style)",
			paths: []string{"docker-compose.infra.yml", "docker-compose.release.yml", "docker-compose.ci.yaml"},
			want:  []string{"docker-compose.infra.yml", "docker-compose.release.yml", "docker-compose.ci.yaml"},
		},
		{
			name:  "terraform blanket extension",
			paths: []string{"infra/main.tf", "infra/variables.tf", "infra/terraform.tfvars"},
			want:  []string{"infra/main.tf", "infra/variables.tf"},
		},
		{
			name:  "helm chart anywhere",
			paths: []string{"charts/myapp/Chart.yaml", "charts/myapp/values.yaml"},
			want:  []string{"charts/myapp/Chart.yaml"},
		},
		{
			name:  "kubernetes by directory name",
			paths: []string{"k8s/deployment.yaml", "deploy/service.yml", "manifests/ingress.yaml"},
			want:  []string{"k8s/deployment.yaml", "deploy/service.yml", "manifests/ingress.yaml"},
		},
		{
			name:  "kubernetes directory match at any depth",
			paths: []string{"infra/k8s/prod/deployment.yaml"},
			want:  []string{"infra/k8s/prod/deployment.yaml"},
		},
		{
			name:  "yaml outside known infra dirs is ignored",
			paths: []string{"config/database.yaml", ".github/workflows/ci.yml", "mkdocs.yml"},
			want:  nil,
		},
		{
			name:  "compose-like name but not actually a compose file is not falsely excluded from other checks",
			paths: []string{"composer.json", "recompose.yaml"},
			want:  nil,
		},
		{
			name:  "non-yaml files under k8s-like dir names still ignored unless yaml",
			paths: []string{"k8s/README.md", "k8s/kustomization.txt"},
			want:  nil,
		},
		{
			name:  "unrelated files ignored",
			paths: []string{"src/index.js", "package.json", "go.mod", "LICENSE"},
			want:  nil,
		},
		{
			name: "budget caps at 12 when no manifest slots used",
			paths: []string{
				"k8s/1.yaml", "k8s/2.yaml", "k8s/3.yaml", "k8s/4.yaml", "k8s/5.yaml",
				"k8s/6.yaml", "k8s/7.yaml", "k8s/8.yaml", "k8s/9.yaml", "k8s/10.yaml",
				"k8s/11.yaml", "k8s/12.yaml", "k8s/13.yaml",
			},
			manifestSlotsUsed: 0,
			want: []string{
				"k8s/1.yaml", "k8s/2.yaml", "k8s/3.yaml", "k8s/4.yaml", "k8s/5.yaml",
				"k8s/6.yaml", "k8s/7.yaml", "k8s/8.yaml", "k8s/9.yaml", "k8s/10.yaml",
				"k8s/11.yaml", "k8s/12.yaml",
			},
		},
		{
			name: "budget shrinks when manifest slots used",
			paths: []string{
				"k8s/1.yaml", "k8s/2.yaml", "k8s/3.yaml", "k8s/4.yaml", "k8s/5.yaml",
				"k8s/6.yaml", "k8s/7.yaml", "k8s/8.yaml",
			},
			manifestSlotsUsed: 5,
			want: []string{
				"k8s/1.yaml", "k8s/2.yaml", "k8s/3.yaml", "k8s/4.yaml", "k8s/5.yaml",
				"k8s/6.yaml", "k8s/7.yaml",
			},
		},
		{
			name:              "budget exhausted when manifest slots fill the pool",
			paths:             []string{"Dockerfile", "docker-compose.yml"},
			manifestSlotsUsed: 12,
			want:              nil,
		},
		{
			name:              "manifest slots exceeding pool still yields empty, not negative",
			paths:             []string{"Dockerfile"},
			manifestSlotsUsed: 20,
			want:              nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindInfraConfigPaths(tc.paths, tc.manifestSlotsUsed)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindInfraConfigPaths(%v, %d) = %v, want %v", tc.paths, tc.manifestSlotsUsed, got, tc.want)
			}
		})
	}
}

func TestInfraConfigKind(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"Dockerfile", "docker"},
		{"Dockerfile.dev", "docker"},
		{"docker-compose.yml", "compose"},
		{"infra/main.tf", "terraform"},
		{"charts/myapp/Chart.yaml", "helm"},
		{"k8s/deployment.yaml", "kubernetes"},
	}

	for _, tc := range cases {
		if got := infraConfigKind(tc.path); got != tc.want {
			t.Errorf("infraConfigKind(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
