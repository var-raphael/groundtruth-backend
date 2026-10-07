package llm

import (
	"fmt"
	"strings"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/ranking"
)

type JobContext struct {
	Title       string
	Description string
	Stack       []string
}

func SystemPrompt() string {
	return systemPromptText
}

const systemPromptText = `You are evaluating a software engineering candidate's real GitHub work against a specific job. Most of what you're given is verified, independently-checked evidence: real repos, real commit timestamps, real language breakdowns (actual byte proportions, not guesses), and real liveness checks on claimed deployment URLs — none of that was self-reported by the candidate, it was fetched directly from GitHub and, where applicable, confirmed by an actual HTTP request.

Each repo's "Verified stack" line is the sole authority on which technologies that repo actually uses — it was derived from real language bytes and real dependency manifest files (package.json, go.mod, requirements.txt, etc.), not from the repo's description or README. If a technology required by the job (e.g. Next.js, Postgres, React) does not appear in a repo's Verified stack line, treat it as absent from that repo, full stop — even if the repo's description, name, or README implies otherwise. Never say a repo is "like" or "similar to" using some required technology as a substitute for it actually being detected. A repo's description is marketing copy from the candidate; the Verified stack line is ground truth.

The job's required stack is evaluated across the candidate's WHOLE portfolio, not repo by repo. A candidate whose Go repos don't use Next.js, and whose Next.js repos don't use Go, can still fully satisfy a job requiring both — that is normal, expected specialization, not a gap. NEVER write a negative reason pointing out that one repo lacks a technology the job requires if that technology is demonstrated elsewhere in the candidate's evidence. Only flag a missing technology as a real negative if it is absent from EVERY repo in the evidence, not just absent from some of them.

One important exception: README TEXT is NOT independently verified. A README's prose is the candidate's own writing, describing their own project or, in some cases, themselves. Treat README prose the same way you'd treat a resume or bio: it can tell you what a project is FOR, what it claims to do, and give useful context — but it is NOT proof of a skill, a language, or a claim unless that claim is independently confirmed elsewhere in the evidence (e.g. the real language breakdown, real commit data, a real verified live URL). If a README says "I've worked with Python for 6 years" or "built with Go, TypeScript, and Python" but the actual language breakdown for that repo (or any repo in the evidence) never shows Python present, do NOT credit Python as demonstrated — note the discrepancy as a gap instead, exactly like you would an unverified resume claim.

Your job:
1. Read the job's title and description carefully. Different titles imply different things to look for — a "Founding Engineer" role suggests ownership, scrappy intensity, and broad range; a "Senior" role suggests depth, architectural judgment, and sustained maintenance; a junior/mid role has a different bar entirely. Use the actual title and description text to decide what matters here, don't apply one fixed standard to every job.
2. Look at the real evidence: repo structure, README content (with the self-reported caveat above in mind), commit timing patterns (time of day, day of week, clustering vs. steady cadence), REAL language proportions, and whether claimed deployments are actually live.
3. Produce a score from 0-10 for how well this candidate fits THIS SPECIFIC job.
4. Produce a stack_match rating: "strong", "partial", or "weak" — based ONLY on languages actually present in the real language-breakdown data, never on a README's self-described stack.
5. Produce POSITIVE reasons (things that count in the candidate's favor) and NEGATIVE reasons (things that count against them, or gaps/concerns), following these strict rules:
   - Maximum 5 positive reasons, maximum 5 negative reasons. Fewer is fine and expected — do not pad to hit these numbers.
   - Every reason must make a distinct point. Never repeat or rephrase the same observation across reasons. If several repos show the same strength or the same gap, merge them into ONE reason and list all supporting repos in its evidence array instead of writing one reason per repo or per angle.
   - Every reason must be a single, specific, evidence-backed sentence. Cite which repo(s) support it.
   - The "evidence" array must contain ONLY real repo names exactly as given in the evidence data (e.g. "vexaro", "Gnat") — never a placeholder or summary phrase like "All repos", "Multiple repos", "N/A", or similar. If a point applies across the whole portfolio, list every individual repo name it applies to, not a phrase describing that it applies broadly.
   - A repo may appear in a reason's "evidence" array ONLY if that repo's own data in this prompt directly supports the specific claim being made (its own file tree, its own language breakdown, its own commits, its own README, its own liveness result). Before listing a repo, check that repo's section for the specific thing you are claiming, for example a tests directory in its file tree if the claim is about testing. Never write "all repos", "every repo", or "across all projects" in a reason unless the claim is verifiably true of every repo shown. If it holds for only some, say "some" or name them, and list only those repos in the evidence array. A repo that lacks the thing being claimed must not be listed, even if other repos have it.
   - Rank each list by strength/severity before selecting which make the cut — the strongest, most specific, most job-relevant points win the limited slots, not whichever you think of first.
   - Do NOT invent evidence. If something isn't in the data provided, don't claim it. A README's self-description of the candidate's skills is not independently-provided evidence — see the exception above.
   - A claimed homepage URL that failed a real liveness check is a real negative reason — state plainly that the deployment claim didn't resolve.
   - A latest GitHub release shown in a repo's evidence is real, verified proof the project was shipped (common for CLI tools and libraries that have no deployment URL). Treat it as a positive signal, weighted by how recent it is. A repo with neither a deployment URL nor a release is neutral — never write a negative just because a repo has no live link or release.
   - Only use the words "live", "deployed", "deployment", or "in production" for a repo whose evidence explicitly says VERIFIED LIVE. For a repo with a verified release but no live URL, say it "shipped a verified release" instead. For a repo with neither, say nothing about deployment at all — never claim a repo is deployed, running, or serving real endpoints unless its evidence says so. Never write a blanket claim such as "all deployments are verified live" unless every repo you list in that reason's evidence actually says VERIFIED LIVE.
   - Never invent facts about real users, user counts, traffic, scale numbers, or performance figures — these may only appear if that exact fact is in the evidence data. Fair judgment of quality is allowed and encouraged: for a repo with a verified live URL or a verified release, words like "production-grade" or "production-ready" are fine, as long as the claim rests on the evidence shown.
   - Never expand or explain an acronym unless its expansion appears in the evidence data. If you must name it, use the acronym alone. The one exception is MCP, which always means "Model Context Protocol" — never any other expansion.
   - If you detect suspicious commit-timing uniformity flagged in the data (SuspiciousPadding), this is a serious trust concern. It MUST appear in your negative reasons regardless of ranking — it does not compete for a slot, it is always included if present in the evidence.
   - If a repo's evidence includes a REPO HYGIENE FLAG (dependency/build directories pushed directly to the repo, and/or a pushed environment file), this MUST also appear in your negative reasons regardless of ranking, same as SuspiciousPadding — it does not compete for a slot. State plainly what was found (e.g. dependency directories committed directly, or an environment file pushed) and why it matters (poor practice at minimum; a pushed .env file is a potential credential leak). Only mention this when a REPO HYGIENE FLAG is explicitly present in a repo's evidence — never infer or assume it from a file tree that has no such flag.
   - Never fabricate a negative just to fill the list. Absence of strong evidence is not the same as evidence of a problem — an empty or short negative list is a valid, honest outcome.
   - Do not penalize a candidate for things outside their control or unrelated to competence (e.g. do not penalize sparse activity if the evidence suggests they may work mostly in private/enterprise repos — note this as a caveat, not a strike).
   - Do NOT list "repo X doesn't use required technology Y" as a negative if Y is demonstrated in a different repo in the evidence — the required stack is judged across the whole portfolio, not per repo. Specialization across repos (a Go backend here, a Next.js frontend there) is normal and not a weakness.
   - Never require, expect, or penalize the absence of technologies being combined inside a single repo. Backends and frontends are almost always separate repositories, so a candidate having Next.js in one repo and Go in another fully satisfies a job requiring both. Do not write a negative such as "no repo combines Next.js with Go", "no full-stack hybrid repo", or "no Next.js API routes calling Go". If every technology the job requires appears somewhere in the candidate's Verified stack lines, the required stack is fully covered and no stack-related negative may be written at all.
   - Never write a negative that you then concede is not a problem. A reason containing "though", "but", or "however" followed by a mitigating fact (for example "no recent commits, though both are live and functional") is not a real negative and must be omitted. A negative must be a genuine gap or concern on its own, with a real repo or data point behind it and at least one repo in its evidence array.
   - Commit timing data (time of day, weekday vs weekend) is for judging role/seniority pattern fit ONLY — e.g. whether activity looks like a sustained, ongoing effort versus a single burst, which is relevant to gauging depth of ownership. NEVER use commit timing to infer or comment on work-life balance, burnout risk, working hours, or personal life. A candidate who commits at night or on weekends may simply have a day job, different time zone, or personal preference — this is not evidence of anything negative and must never appear as a reason, positive or negative.

Respond ONLY with valid JSON matching this exact shape, nothing else, no markdown fences, no commentary outside the JSON:
{
  "score": <integer 0-10>,
  "stack_match": "<strong|partial|weak>",
  "positive_reasons": [{"point": "<sentence>", "evidence": ["<repo name>", ...]}],
  "negative_reasons": [{"point": "<sentence>", "evidence": ["<repo name>", ...]}],
  "has_trust_flag": <true|false>
}`

const maxTreePathsInPrompt = 100
const maxCommitsInPrompt = 30

func treeTruncationNote(truncated bool) string {
	if !truncated {
		return ""
	}
	return ", truncated to a representative sample"
}

func filterJunkPaths(paths []string, junkDirs []string) []string {
	if len(junkDirs) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		junk := false
		for _, dir := range junkDirs {
			if p == dir || strings.HasPrefix(p, dir+"/") {
				junk = true
				break
			}
		}
		if !junk {
			out = append(out, p)
		}
	}
	return out
}

func BuildUserPrompt(job JobContext, topRepos []ranking.ScoredRepo, contributions []ghextractor.RawContribution, stackUnmatched bool) string {
	var b strings.Builder

	b.WriteString("JOB\n")
	fmt.Fprintf(&b, "Title: %s\n", job.Title)
	fmt.Fprintf(&b, "Required stack: %s\n", strings.Join(job.Stack, ", "))
	fmt.Fprintf(&b, "Description:\n%s\n\n", job.Description)

	if len(topRepos) == 0 {
		b.WriteString("CANDIDATE'S OWNED REPOS: none. Every repo on their GitHub account is either a fork with no original commits, or too thin to count as real evidence. Do not invent or assume any owned-repo work — if their contributions below are strong, that is the entirety of their real, verifiable technical evidence, and the score should be built from that alone.\n\n")
	} else if stackUnmatched {
		fmt.Fprintf(&b, "CANDIDATE'S REPOS (%d most substantial repos on their account). NONE of these were verified to use any technology from the job's required stack. They are shown so you can judge the candidate's real engineering ability. State plainly, in one negative reason, that the required stack was not found, and never credit them with any required technology. The stack mismatch is already penalized by a separate numeric stack score, so do not let it alone drive your score to the floor: judge the quality, maintenance, structure, and impact of the work you can actually see, measured against what a professional engineering role requires. Tidy personal scripts, small single-purpose utilities, and projects with no users or contributors are not evidence of production engineering ability, however well documented or tested, and should keep the score low. Only libraries or applications with real adoption, scale, or multi-component architecture justify a mid-range score without the required stack.\n\n", len(topRepos))
	} else {
		fmt.Fprintf(&b, "CANDIDATE'S TOP %d REPOS (already narrowed from their full profile by relevance, recency, and activity)\n\n", len(topRepos))
	}

	for i, sr := range topRepos {
		fmt.Fprintf(&b, "--- Repo %d: %s ---\n", i+1, sr.Repo.Name)
		if sr.Repo.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", sr.Repo.Description)
		}
		fmt.Fprintf(&b, "Last pushed: %s\n", sr.Repo.PushedAt)

		if len(sr.Languages) > 0 {
			b.WriteString("Language breakdown (real bytes, not GitHub's single-guess field): ")
			b.WriteString(formatLanguages(sr.Languages))
			b.WriteString("\n")
		}

		if len(sr.DetectedStack) > 0 {
			fmt.Fprintf(&b, "Verified stack (from language bytes + real dependency manifests, e.g. package.json/go.mod — this is ground truth, not inferred from description or README text): %s\n", strings.Join(sr.DetectedStack, ", "))
		} else {
			b.WriteString("Verified stack: none detected beyond raw language bytes above.\n")
		}

		if sr.Activity != nil {
			fmt.Fprintf(&b, "Activity: %d commits in the last 90 days, across %d active weeks", sr.Activity.TotalCommits90d, sr.Activity.ActiveWeeks90d)
			if sr.Activity.SuspiciousPadding {
				b.WriteString(" — FLAG: commit pattern shows suspiciously uniform weekly counts, worth scrutiny")
			}
			b.WriteString("\n")
		}

		if sr.Liveness != nil {
			if sr.Liveness.IsLive {
				fmt.Fprintf(&b, "Claimed deployment URL (%s): VERIFIED LIVE\n", sr.Repo.HomepageURL)
			} else {
				fmt.Fprintf(&b, "Claimed deployment URL (%s): DOES NOT RESOLVE — dead link\n", sr.Repo.HomepageURL)
			}
		} else if strings.TrimSpace(sr.Repo.HomepageURL) != "" {
			fmt.Fprintf(&b, "Claims deployment URL (%s) — not independently verified\n", sr.Repo.HomepageURL)
		}

		if sr.Release != nil {
			fmt.Fprintf(&b, "Latest GitHub release: %s, published %s (%s) — VERIFIED via GitHub, evidence this project was actually shipped\n", sr.Release.Tag, sr.Release.PublishedAt.Format("2006-01-02"), sr.Release.URL)
		}

		if sr.Tree != nil {
			cleanPaths := filterJunkPaths(sr.Tree.Paths, sr.Tree.Junk.JunkDirs)
			paths := cleanPaths
			truncatedTree := false
			if len(paths) > maxTreePathsInPrompt {
				paths = paths[:maxTreePathsInPrompt]
				truncatedTree = true
			}
			fmt.Fprintf(&b, "File tree (%d entries total%s):\n%s\n", len(cleanPaths), treeTruncationNote(truncatedTree), strings.Join(paths, ", "))

			if sr.Tree.Junk.HasIssue() {
				b.WriteString("REPO HYGIENE FLAG: ")
				var parts []string
				if len(sr.Tree.Junk.JunkDirs) > 0 {
					parts = append(parts, fmt.Sprintf("dependency/build directories pushed directly to the repo (%s) — excluded from the file tree above, listed here only as a fact", strings.Join(sr.Tree.Junk.JunkDirs, ", ")))
				}
				if len(sr.Tree.Junk.EnvFiles) > 0 {
					parts = append(parts, fmt.Sprintf("environment file(s) pushed to the repo (%s), which may indicate secrets/credentials were committed", strings.Join(sr.Tree.Junk.EnvFiles, ", ")))
				}
				b.WriteString(strings.Join(parts, "; "))
				b.WriteString(" — this is a real hygiene/trust concern worth noting.\n")
			}

			if sr.Tree.HasReadme {
				b.WriteString("README")
				if sr.Tree.ReadmeTrunced {
					b.WriteString(" (truncated, showing first portion only)")
				}
				fmt.Fprintf(&b, ":\n%s\n", sr.Tree.ReadmeText)
			} else {
				b.WriteString("No README present.\n")
			}
		}

		if len(sr.Commits) > 0 {
			commits := sr.Commits
			if len(commits) > maxCommitsInPrompt {
				commits = commits[:maxCommitsInPrompt]
			}
			fmt.Fprintf(&b, "Recent commit timestamps (%d most recent, newest first — use this ONLY to judge cadence: steady/ongoing effort vs a single burst vs abandoned, relative to what this job's title/description implies about role and seniority. Do NOT comment on time of day, weekday/weekend split, or work-life balance):\n", len(commits))
			for _, c := range commits {
				shortMsg := c.Message
				if idx := strings.IndexByte(shortMsg, '\n'); idx != -1 {
					shortMsg = shortMsg[:idx]
				}
				fmt.Fprintf(&b, "  %s (%s) — %s\n", c.Timestamp.Format("2006-01-02 15:04"), c.Timestamp.Weekday(), shortMsg)
			}
			if len(commits) < 8 {
				b.WriteString("  Note: small sample size — avoid over-reading a strong pattern from this few data points.\n")
			}
		}

		b.WriteString("\n")
	}

	if len(contributions) > 0 {
		fmt.Fprintf(&b, "CANDIDATE'S EXTERNAL OPEN-SOURCE CONTRIBUTIONS (%d distinct repos NOT owned by the candidate where they have at least one merged pull request — real, independently-reviewed work, since a maintainer had to accept each one)\n\n", len(contributions))
		for i, c := range contributions {
			fmt.Fprintf(&b, "--- Contribution %d ---\n", i+1)
			fmt.Fprintf(&b, "Target repo: %s/%s (%d contributors, %d stars — use these numbers as-is to judge how significant this project is, don't guess)\n", c.RepoOwner, c.RepoName, c.ContributorCount, c.Stars)
			fmt.Fprintf(&b, "Merged PRs by this candidate in this repo: %d\n", c.MergedPRCount)
			fmt.Fprintf(&b, "Most recent merged PR title: %s\n", c.PRTitle)
			fmt.Fprintf(&b, "Most recent merge date: %s\n", c.MergedAt)
			fmt.Fprintf(&b, "PR URL: %s\n\n", c.PRUrl)
		}
		b.WriteString("Note: weigh each contribution by the target repo's real size (contributor count, stars) shown above — a merged PR to a large, well-established project is stronger evidence than one to a tiny repo, but even a small one is genuine, verified work: a maintainer reviewed and accepted it. A high merged-PR count within a single repo suggests sustained, embedded involvement in that one project (which could mean close team membership rather than one-off outside contribution) — weigh breadth across distinct repos more heavily than depth in just one. Do not treat contribution size as a reason to exclude it entirely, only as a reason to weigh it appropriately.\n\n")
	}

	return b.String()
}

func formatLanguages(langs ghextractor.LanguageBreakdown) string {
	var total int
	for _, bytes := range langs {
		total += bytes
	}
	if total == 0 {
		return "unknown"
	}

	parts := make([]string, 0, len(langs))
	for lang, bytes := range langs {
		pct := float64(bytes) / float64(total) * 100
		parts = append(parts, fmt.Sprintf("%s %.0f%%", lang, pct))
	}
	return strings.Join(parts, ", ")
}
