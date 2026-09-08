# groundtruth

AI-powered technical hiring verification tool. Given a job (title, description, required stack) and a candidate's GitHub username, it produces a 0-10 score with a written breakdown of real, verified evidence — real repos, real commit history, real language breakdowns, real liveness checks on claimed deployments, real merged PRs — not self-reported claims.

## Core philosophy

- **Mechanical scoring and LLM judgment are kept separate, and deliberately not double-counted.** Stack Match, Evidence Strength, and Contributions are all computed by plain Go code from verified GitHub data. LLM Judgment is Mistral's own qualitative read. The final score is a weighted average of all four.
- **Owned-repo evidence and external-contribution evidence are two separate signals, never blended.** A candidate can be strong on one and weak on the other (see: Raphael vs Brendan below), and that's the point — it tells a real story instead of hiding it behind one number.
- **Mechanical rules stay narrow and legible; judgment calls belong to the LLM.** E.g. we don't try to mechanically detect "is this good code" — we hand the LLM real tree/README/commit data and let it reason, with the system prompt guiding what it should and shouldn't infer (see the em-dash and work-life-balance rules in `prompt.go` for how specific this got).
- **No comments in code.** The user explicitly asked for comment-free Go files going forward. Keep it that way in future edits.

## Architecture, file by file

### `cmd/testrun/main.go`
The manual test harness. Hardcodes a GitHub username and a `llm.JobContext`, runs the full scoring pipeline, writes the score breakdown + full reasoning JSON to `test.txt`. This is how every fix in this project has been validated — swap `username` and the `CandidateInfo` block to test a different profile, then `go run ./cmd/testrun`.

No outreach call in here anymore (removed deliberately — outreach is on-demand via the API, not generated during scoring).

### `internal/extractor/github/` — raw GitHub data fetching
- `client.go` — GitHub client constructor.
- `repos.go` — `FetchOwnedRepos`, `RawRepo` struct.
- `tree.go` — `FetchTree`, `TreeSummary` (file paths, README presence/truncation).
- `languages.go` — `FetchLanguages`, `LanguageBreakdown` (raw byte counts per language).
- `activity.go` — `FetchActivity` (commit counts, active weeks, suspicious-padding detection for future-dated/fake-looking commit patterns).
- `commits.go` — `FetchRecentCommitTimings`. **Filters by `Author: owner`** so only commits actually authored by the candidate are used (fixed — previously pulled all commits on the branch regardless of author, which could count a collaborator's or bot's commits as the candidate's own).
- `forkstatus.go` — `FetchForkStatus`. Compares a fork against its parent, returns `CommitsAhead` and `LinesChanged` (fixed — `LinesChanged` is new, computed from the compare API's file-level diffs).
- `liveness.go` — `CheckLiveness`, real HTTP check (with retries for cold starts) on a claimed homepage URL.
- `contributions.go` — `FetchExternalContributions`. Searches `author:{username} type:pr is:merged`, **dedupes to distinct repos before doing expensive per-repo lookups** (fixed — previously called `Repositories.Get` + contributor-count for every single PR, which was ~1,344 wasted API calls for a candidate with 672 PRs). Caps output at the **top 5 distinct repos by contributor count**. Excludes the candidate's own repos.

### `internal/ranking/` — mechanical per-repo scoring and selection
- `filter.go` — `FilterOwnedRepos`. Drops the GitHub profile-README repo. For forks, now requires **both** `CommitsAhead >= 3` **and** `LinesChanged >= 50` to count as owned work (fixed — was `CommitsAhead >= 1`, meaning a single trivial commit on a huge forked codebase used to count as fully-owned evidence).
- `score.go` — `ScoreRepo`, `ScoredRepo` struct, `stackMatchScore` (per-repo language presence, 3% floor — used only for `selectTopK`'s stack-completion phase, a looser bar than the final stack-match calculation). `NonStackScore()` rescales a repo's non-stack score onto a shared 0-60 range based on whether it has a `HomepageURL` — a repo with no claimed deployment isn't penalized for not competing in the liveness category.
- `topk.go` — `BuildTopRepos`, `selectTopK`. `defaultTopK = 6`. Selection is two-phase: (1) seat the single best-scoring repo per required stack language (guarantees stack-relevant repos survive score noise), (2) fill remaining slots by score — but only with repos that clear `hasRealSubstanceForFiller` (8+ tree files, 2+ languages), so thin single-purpose repos (a 100%-HTML landing page, e.g.) can't occupy a slot and drag down Evidence Strength.

### `internal/scoring/` — combining everything into a final score
- `finalscore.go` — `ComputeFinalScore`. Four weighted components:
  - **Stack Match**: a language counts as covered only if some repo has **40%+ presence of it AND 8+ tree files** (fixed twice — originally a naive 3% byte-floor across any repo caused false positives, e.g. a single vendored file registering as "Go", or a 100%-Python 3.6KB single-file script counting as real Python evidence).
  - **Evidence Strength**: average of `NonStackScore()` across top-K repos.
  - **Contributions**: `contributionsScore` — only counts repos with `ContributorCount >= 5` (excludes low-credibility repos where self-merging is trivial), log-scaled and summed per distinct repo (fixed — previously summed per raw PR, so one person with 672 PRs into a handful of repos scored the same as if they'd made 672 genuinely independent external contributions).
  - **LLM Judgment**: Mistral's own score.
- `pipeline.go` — `ScoreCandidate` (orchestrates fetch → rank → contribution fetch → LLM call → evidence resolution) and `BuildReport` (assembles the final `models.CandidateReport`). Early-return for "insufficient evidence" now requires **both** `topRepos` and `contributions` to be empty (fixed — previously returned an empty/zero result if `topRepos` alone was empty, even if the candidate had strong external contributions, which meant a "Pull Shark"-style profile with zero owned repos got a hollow 0/0/0/0 instead of a real LLM judgment based on their contributions).

### `internal/llm/` — the Mistral integration
- `client.go` — `Client.CompleteJSON`. Model pinned to `ministral-14b-2512` (fixed — was `mistral-small-latest`, which hit a stuck/degraded rate limit one morning that a fresh key on a fresh account couldn't clear either; confirmed via bare curl tests that it was model-specific, not account-specific). Retries with backoff on 5xx/network errors only — **429 fails immediately**, no retry (a hard per-minute rate limit can't be outlasted by a few seconds of backoff, and retrying just burns more quota).
- `prompt.go` — `SystemPrompt`, `BuildUserPrompt`. Contains the accumulated tone/behavior rules:
  - No inferring work-life balance, burnout, or personal-life commentary from commit timing (timing is for judging cadence/ownership-depth only).
  - No em dashes or en dashes anywhere in generated text.
  - Tree paths capped at 100 per repo, commits capped at 30 per repo (prevents a large/prolific candidate's prompt from exceeding the model's context window — this actually happened with a 45-repo profile and threw a 400 "prompt too long" error).
  - Explicit handling for zero-owned-repos candidates (states plainly there are none, so the LLM doesn't hallucinate repo evidence and knows to build its judgment from contributions alone).
  - Contribution weighting note: breadth across distinct repos should be weighed more heavily than raw depth in one repo.
- `reasons.go` — `EnforceReasonLimits` (caps reason list lengths, **strips markdown** `**`/`__`/`` ` `` from reason text — the model occasionally bolds text despite not being asked to), `VerifyTrustFlagHonored`, `ResolveEvidence` (matches reason evidence names against real repo/contribution URLs so citations are never invented).
- `schema.go` — `ParseJobReasoning` and the raw/resolved reasoning types.

### `internal/outreach/template.go`
`BuildDraft` — LLM-drafted (not templated) outreach email. Takes only `PositiveReasons` + evidence from a `CandidateReport` (deliberately excludes negative reasons from the drafting prompt). Rules baked into its system prompt: founder voice not recruiter-speak, 3-5 sentences, no invented facts, no em dashes, opens with "Hi {first name}," greeting, signs off "Best," with nothing after, never mentions the candidate's score/ranking. **Generated on-demand only** — not called during scoring, only when a recruiter clicks "draft email" in the UI (matches the existing frontend mockup's `DraftEmailButton` → `EmailPreviewModal` flow).

### `internal/api/handlers/outreach.go`
Minimal `net/http` handler wiring `outreach.BuildDraft` to an HTTP endpoint. **Not yet wired to an actual router** — no router framework has been chosen yet, and the request shape (full `CandidateReport` + `Job` in the body) is a placeholder pending the database design, since the real version should probably take just IDs and fetch both server-side.

### `internal/models/` — shared data shapes
`report.go` (`CandidateReport`, `ReasoningSummary`, `ContributionSummary` — now has `MergedPRCount`), `candidate.go`, `evidence.go`, `job.go`.

### Empty / not-yet-built
- `internal/db/` — nothing yet. **This is the next major piece of infrastructure.**
- `internal/worker/scan.go` — 0 bytes.
- `pkg/config/config.go` — 0 bytes.
- `cmd/api/main.go` — 0 bytes.
- `internal/api/handlers/{apply,candidates,jobs,auth}.go` — 0 bytes.
- `internal/api/middleware/{auth,ratelimit}.go` — 0 bytes.

## Known, deliberately deferred gaps

These were discussed and consciously not addressed — not oversights, but decisions to revisit only if a real test case demands it:

- **Stack Match doesn't consider contribution-repo language**, only owned-repo language. A candidate with zero owned repos but substantial merged PRs in Go would still show Go stack match as uncovered. Decided against folding this in — it would blend two signals that are more useful kept separate; the LLM's own prose reasoning already picks up this nuance case by case (see Brendan's report, where it explicitly credited his Go contribution work as a separate positive reason).
- **Vendored/generated code in a repo's tree isn't filtered out.** Decided this isn't worth solving — top-K is capped at 6 repos regardless of total repo count, and per-repo tree/commit caps (100 paths, 30 commits) already bound prompt size regardless of how large any single repo's tree is. If someone commits `node_modules/`, that's itself a mild, honest negative signal (poor `.gitignore` hygiene), not something to hide from the LLM.

## Architecture decisions made but not yet built

- **Worker concurrency model** (discussed, not implemented): GitHub-stage fetching (repos, trees, activity, liveness) should run wide and concurrent per-user, since each user's own OAuth token means no shared rate-limit ceiling across users. The AI-stage (Mistral calls) should go through a **durable, Postgres-backed queue**, not a worker-pool-with-capped-concurrency — because the real constraint is Mistral's own TPM/RPS ceiling (shared across all users), not Go's concurrency limits. Two Mistral API keys on two separate accounts are available for rotation to roughly double throughput. Queue should be additive-friendly: adding a key or a paid tier later should just mean the queue drains faster, no redesign needed.
- **Outreach endpoint request shape**: deliberately left unresolved pending the database. Likely resolution once persistence exists: send `candidateId` + `jobId`, have the backend fetch both, rather than the frontend shuttling the full report through the request body.
- **Supabase Postgres** is the intended database (not SQLite — deliberately ruled out due to single-writer lock contention between concurrent GitHub-stage and AI-stage workers).

## Test profiles used and what each one validated

Useful to re-run against these if you touch scoring logic — each one previously exposed a real bug:

- **`var-raphael`** — the primary/baseline profile. Strong, legitimate Go/TypeScript/Python work across several owned repos, one real external contribution (sourcebot). Used to confirm fixes don't regress a genuinely strong candidate.
- **`olly-techie`** — JS/PHP-heavy profile with several thin, HTML-wrapper-style repos. Exposed the filler-repo top-K problem and an early stack-match false positive (a phantom Go match with zero real Go anywhere).
- **`brendan-kellam`** — sourcebot core contributor/maintainer, 45 owned repos (mostly stale since 2017-2021), 672 raw merged PRs. Exposed: the prompt-too-long 400 error (fixed via tree/commit caps), the contribution-count-inflation bug (fixed via distinct-repo dedup + contributor floor + top-5 cap), and the real architectural insight that owned-repo strength and contribution strength can be wildly lopsided in either direction for a genuinely accomplished engineer.
- **`drew-u410`** — "Pull Shark" profile, all forks, zero owned repos, but 5 legitimate external contributions. Exposed the empty-topRepos early-return bug (was skipping the LLM call entirely whenever owned repos were empty, even with strong contribution evidence available).

## How to run a test

```
go run ./cmd/testrun
```

Reads `.env` for `GITHUB_TOKEN` and `MISTRAL_API_KEY`. Writes output to `test.txt` in the project root. Edit the `username` variable and the `scoring.CandidateInfo` block in `main.go` to point at a different GitHub profile.
