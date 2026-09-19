package llm

import (
	"fmt"
	"strings"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
)

func StackDetectionSystemPrompt() string {
	return stackDetectionSystemPromptText
}

const stackDetectionSystemPromptText = `You detect the real technologies used in a software repository, given its language byte breakdown and the raw contents of its dependency manifest files (package.json, go.mod, requirements.txt, composer.json, etc.).

Your job is to produce a flat list of specific technology names actually in use — languages, frameworks, and databases — based only on what the evidence shows. Rules:

1. Use the language byte breakdown as ground truth for which programming languages are present.
2. Use the manifest file contents to detect frameworks and databases that language bytes alone cannot reveal — e.g. a "next" dependency in package.json means Next.js is in use even if the language breakdown just shows TypeScript; a Postgres driver import (pg, psycopg2, github.com/lib/pq, gorm.io/driver/postgres, github.com/jackc/pgx) means PostgreSQL is in use even though "Postgres" never appears as a language.
3. Use normalized, human-recognizable names a recruiter would type into a job posting: "React", "Next.js", "Django", "Laravel", "PostgreSQL", "MySQL", "MongoDB", "Redis", "Go", "Python", "TypeScript" — not raw package names, not file names.
4. Do not invent a technology that isn't evidenced by either the language bytes or an actual dependency in the manifest contents provided. If a manifest is absent or empty, rely on language bytes alone.
5. Frontend meta-frameworks (Next.js, Remix, Gatsby, Nuxt) all depend on their base framework (React or Vue) — when one of these is present, include both the meta-framework and its base framework (e.g. both "Next.js" and "React").
6. Deduplicate. Each technology appears once in the output list.

Respond ONLY with valid JSON matching this exact shape, nothing else, no markdown fences, no commentary outside the JSON:
{
  "stack": ["<technology name>", ...]
}`

func BuildStackDetectionPrompt(languages ghextractor.LanguageBreakdown, manifestFiles []ghextractor.ManifestFile) string {
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
		b.WriteString("No dependency manifest files found in this repo's tree.\n")
	}

	return b.String()
}

func StackDetectionBatchSystemPrompt() string {
	return stackDetectionBatchSystemPromptText
}

const stackDetectionBatchSystemPromptText = `You detect the real technologies used across multiple software repositories belonging to the same candidate, given each repo's language byte breakdown and the raw contents of its dependency manifest files (package.json, go.mod, requirements.txt, composer.json, etc.).

For each repo, produce a flat list of specific technology names actually in use — languages, frameworks, and databases — based only on what the evidence shows for that repo. Rules, applied independently per repo:

1. Use the language byte breakdown as ground truth for which programming languages are present.
2. Use the manifest file contents to detect frameworks and databases that language bytes alone cannot reveal — e.g. a "next" dependency in package.json means Next.js is in use even if the language breakdown just shows TypeScript; a Postgres driver import (pg, psycopg2, github.com/lib/pq, gorm.io/driver/postgres, github.com/jackc/pgx) means PostgreSQL is in use even though "Postgres" never appears as a language.
3. Use normalized, human-recognizable names a recruiter would type into a job posting: "React", "Next.js", "Django", "Laravel", "PostgreSQL", "MySQL", "MongoDB", "Redis", "Go", "Python", "TypeScript" — not raw package names, not file names.
4. Do not invent a technology that isn't evidenced by either the language bytes or an actual dependency in that specific repo's manifest contents. If a manifest is absent or empty for a repo, rely on language bytes alone for that repo.
5. Frontend meta-frameworks (Next.js, Remix, Gatsby, Nuxt) all depend on their base framework (React or Vue) — when one of these is present, include both the meta-framework and its base framework (e.g. both "Next.js" and "React").
6. Deduplicate within each repo's list. Each technology appears once per repo.
7. Never let one repo's technologies leak into another repo's list. Each repo's evidence is independent even though they belong to the same candidate.

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

		b.WriteString("\n")
	}

	return b.String()
}

type StackDetectionRepoInput struct {
	Name          string
	Languages     ghextractor.LanguageBreakdown
	ManifestFiles []ghextractor.ManifestFile
}
