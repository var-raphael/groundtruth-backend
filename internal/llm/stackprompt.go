package llm

import (
	"fmt"
	"strings"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
)

func StackDetectionSystemPrompt() string {
	return stackDetectionSystemPromptText
}

const stackDetectionSystemPromptText = `You detect the real technologies used in a software repository, given its language byte breakdown, the raw contents of its dependency manifest files (package.json, go.mod, requirements.txt, composer.json, etc.), and the raw contents of its infra/deployment config files (Dockerfile, docker-compose.yml, Terraform, Kubernetes manifests, Helm charts).

Your job is to produce a flat list of specific technology names actually in use — languages, frameworks, databases, and infra services — based only on what the evidence shows. Rules:

1. Use the language byte breakdown as ground truth for which programming languages are present.
2. Use the manifest file contents to detect frameworks and databases that language bytes alone cannot reveal — e.g. a "next" dependency in package.json means Next.js is in use even if the language breakdown just shows TypeScript; a Postgres driver import (pg, psycopg2, github.com/lib/pq, gorm.io/driver/postgres, github.com/jackc/pgx) means PostgreSQL is in use even though "Postgres" never appears as a language.
3. Use the infra/deployment config contents to detect infra services that manifests alone cannot reveal — e.g. a "redis" or "kafka" service image in docker-compose.yml means Redis or Kafka is in use even with no client library in any manifest; a Dockerfile "FROM postgres:15" means PostgreSQL is in use; a Terraform resource like aws_elasticache_cluster, aws_msk_cluster, or aws_db_instance means that AWS-managed service (ElastiCache/Redis, MSK/Kafka, RDS) is in use; an "image: rabbitmq" or "image: confluentinc/cp-kafka" line means that message broker is in use.
4. Use normalized, human-recognizable names a recruiter would type into a job posting: "React", "Next.js", "Django", "Laravel", "PostgreSQL", "MySQL", "MongoDB", "Redis", "Kafka", "RabbitMQ", "AWS", "Go", "Python", "TypeScript" — not raw package names, not file names, not Terraform resource types.
5. Do not invent a technology that isn't evidenced by the language bytes or an actual dependency/service/resource in the manifest or infra config contents provided. If a manifest or infra config is absent or empty, rely on whatever evidence remains.
6. Frontend meta-frameworks (Next.js, Remix, Gatsby, Nuxt) all depend on their base framework (React or Vue) — when one of these is present, include both the meta-framework and its base framework (e.g. both "Next.js" and "React").
7. Deduplicate. Each technology appears once in the output list.

Respond ONLY with valid JSON matching this exact shape, nothing else, no markdown fences, no commentary outside the JSON:
{
  "stack": ["<technology name>", ...]
}`

func BuildStackDetectionPrompt(languages ghextractor.LanguageBreakdown, manifestFiles []ghextractor.ManifestFile, infraFiles []ghextractor.InfraConfigFile) string {
	var b strings.Builder

	if len(languages) > 0 {
		b.WriteString("Language breakdown (real bytes): ")
		b.WriteString(formatLanguages(languages))
		b.WriteString("\n\n")
	} else {
		b.WriteString("Language breakdown: unavailable.\n\n")
	}

	if len(manifestFiles) > 0 {
		fmt.Fprintf(&b, "Dependency manifest files (%d found):\n\n", len(manifestFiles))
		b.WriteString(ghextractor.FormatManifestFiles(manifestFiles))
	} else {
		b.WriteString("No dependency manifest files found in this repo's tree.\n\n")
	}

	if len(infraFiles) > 0 {
		fmt.Fprintf(&b, "Infra/deployment config files (%d found):\n\n", len(infraFiles))
		b.WriteString(ghextractor.FormatInfraConfigFiles(infraFiles))
	} else {
		b.WriteString("No infra/deployment config files found in this repo's tree.\n")
	}

	return b.String()
}

func StackDetectionBatchSystemPrompt() string {
	return stackDetectionBatchSystemPromptText
}

const stackDetectionBatchSystemPromptText = `You detect the real technologies used across multiple software repositories belonging to the same candidate, given each repo's language byte breakdown, the raw contents of its dependency manifest files (package.json, go.mod, requirements.txt, composer.json, etc.), and the raw contents of its infra/deployment config files (Dockerfile, docker-compose.yml, Terraform, Kubernetes manifests, Helm charts).

For each repo, produce a flat list of specific technology names actually in use — languages, frameworks, databases, and infra services — based only on what the evidence shows for that repo. Rules, applied independently per repo:

1. Use the language byte breakdown as ground truth for which programming languages are present.
2. Use the manifest file contents to detect frameworks and databases that language bytes alone cannot reveal — e.g. a "next" dependency in package.json means Next.js is in use even if the language breakdown just shows TypeScript; a Postgres driver import (pg, psycopg2, github.com/lib/pq, gorm.io/driver/postgres, github.com/jackc/pgx) means PostgreSQL is in use even though "Postgres" never appears as a language.
3. Use the infra/deployment config contents to detect infra services that manifests alone cannot reveal — e.g. a "redis" or "kafka" service image in docker-compose.yml means Redis or Kafka is in use even with no client library in any manifest; a Dockerfile "FROM postgres:15" means PostgreSQL is in use; a Terraform resource like aws_elasticache_cluster, aws_msk_cluster, or aws_db_instance means that AWS-managed service (ElastiCache/Redis, MSK/Kafka, RDS) is in use; an "image: rabbitmq" or "image: confluentinc/cp-kafka" line means that message broker is in use.
4. Use normalized, human-recognizable names a recruiter would type into a job posting: "React", "Next.js", "Django", "Laravel", "PostgreSQL", "MySQL", "MongoDB", "Redis", "Kafka", "RabbitMQ", "AWS", "Go", "Python", "TypeScript" — not raw package names, not file names, not Terraform resource types.
5. Do not invent a technology that isn't evidenced by the language bytes or an actual dependency/service/resource in that specific repo's manifest or infra config contents. If a manifest or infra config is absent or empty for a repo, rely on whatever evidence remains for that repo.
6. Frontend meta-frameworks (Next.js, Remix, Gatsby, Nuxt) all depend on their base framework (React or Vue) — when one of these is present, include both the meta-framework and its base framework (e.g. both "Next.js" and "React").
7. Deduplicate within each repo's list. Each technology appears once per repo.
8. Never let one repo's technologies leak into another repo's list. Each repo's evidence is independent even though they belong to the same candidate.

Respond ONLY with valid JSON matching this exact shape, nothing else, no markdown fences, no commentary outside the JSON. Include exactly one entry per repo given, in any order, using the exact repo name provided:
{
  "repos": [
    {"name": "<repo name>", "stack": ["<technology name>", ...]},
    ...
  ]
}`

func BuildStackDetectionBatchPrompt(repos []StackDetectionRepoInput) string {
	var b strings.Builder

	for i, r := range repos {
		fmt.Fprintf(&b, "--- Repo %d: %s ---\n", i+1, r.Name)

		if len(r.Languages) > 0 {
			b.WriteString("Language breakdown (real bytes): ")
			b.WriteString(formatLanguages(r.Languages))
			b.WriteString("\n")
		} else {
			b.WriteString("Language breakdown: unavailable.\n")
		}

		if len(r.ManifestFiles) > 0 {
			fmt.Fprintf(&b, "Dependency manifest files (%d found):\n", len(r.ManifestFiles))
			b.WriteString(ghextractor.FormatManifestFiles(r.ManifestFiles))
		} else {
			b.WriteString("No dependency manifest files found in this repo's tree.\n")
		}

		if len(r.InfraConfigFiles) > 0 {
			fmt.Fprintf(&b, "Infra/deployment config files (%d found):\n", len(r.InfraConfigFiles))
			b.WriteString(ghextractor.FormatInfraConfigFiles(r.InfraConfigFiles))
		} else {
			b.WriteString("No infra/deployment config files found in this repo's tree.\n")
		}

		b.WriteString("\n")
	}

	return b.String()
}

type StackDetectionRepoInput struct {
	Name             string
	Languages        ghextractor.LanguageBreakdown
	ManifestFiles    []ghextractor.ManifestFile
	InfraConfigFiles []ghextractor.InfraConfigFile
}
