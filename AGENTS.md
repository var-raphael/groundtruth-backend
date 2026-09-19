# groundtruth

AI-powered technical hiring verification tool. Given a job (title, description, required stack) and a candidate's GitHub username, it produces a 0-10 score with a written breakdown of real, verified evidence — real repos, real commit history, real language breakdowns, real liveness checks on claimed deployments, real merged PRs — not self-reported claims.

**START HERE:** there is a major, fully-designed-but-not-yet-built pipeline reorder waiting — see "MAJOR PENDING WORK: Pipeline reorder" further down this file. It supersedes some of the "Stack detection" section's current tradeoffs. Read that section before touching `topk.go`, `score.go`, or `finalscore.go`.

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

## Stack detection (DetectedStack)

Stack matching no longer uses raw GitHub language bytes. `ScoredRepo` has a new `DetectedStack []string` field, populated per-repo by combining language byte breakdown + real dependency manifest contents (fetched via GitHub, parsed via `github.com/git-pkgs/manifests`) into one LLM call. This catches frameworks/databases byte proportions can't (e.g. Next.js, Postgres) that plain language detection missed.

- `internal/extractor/github/manifest.go` — `FindManifestPaths` (uses `manifests.Identify` on tree paths, no hardcoded filename list), `FetchManifestFiles` (fetches content via `Repositories.GetContents`, capped at 8 files/20KB each), `FormatManifestFiles`.
- `internal/llm/stackprompt.go` — system/user prompt for the combined detection call.
- `internal/llm/schema.go` — added `DetectedStack` type + `ParseDetectedStack`.
- `internal/ranking/score.go` — added `DetectedStack`/`DetectedStackError` fields, `DetectedStackMatchScore`, `RescoreWithDetectedStack`. The old byte-based `stackMatchScore` is unchanged and still used for `topk.go`'s cheap first-pass selection only.
- `internal/scoring/detectstack.go` — `ApplyDetectedStack`, runs the fetch+LLM step per repo.
- `internal/scoring/finalscore.go` — `languageCoveredAcross` now checks `DetectedStack`, not byte proportions.
- `internal/worker/scan.go` — `runScoring` calls `ApplyDetectedStack` before `ScoreWithEvidence`, so it runs inside the Mistral-concurrency-capped phase, not the uncapped GitHub extraction phase (one LLM call per repo, must stay capped).

`Languages` (raw bytes) is untouched everywhere else — report display, judgment prompt context, phase-one repo selection.

`go.mod` needs `github.com/git-pkgs/manifests v0.12.0` — run `go mod tidy`.

**Fixed this session:** `DetectedStack` was being computed and shown in the report's evidence JSON, but never actually included in the scoring LLM's prompt (`prompt.go`'s `BuildUserPrompt`) — so the LLM was reasoning about stack presence from language bytes and repo descriptions alone, occasionally producing prose that contradicted the mechanical `DetectedStackMatchScore` in the same report (e.g. claiming a repo had a "Next.js-like frontend" when `DetectedStack` showed nothing of the sort). Fixed by adding a "Verified stack" line per repo in the prompt (sourced from `sr.DetectedStack`) plus a hard system-prompt rule: `DetectedStack` is the sole authority on tech presence, description/README claims never override it, no "-like" hedging language allowed. Confirmed fixed via re-running var-raphael against a Next.js/Go/Postgres job — negative reasons now correctly and consistently state "no evidence of Next.js" instead of contradicting themselves.

Not yet done: `topk.go`'s `selectTopK`/`repoCoversLanguage` still seat repos using byte-based matching, not `DetectedStack` — a deliberate tradeoff to keep the uncapped extraction phase free of LLM calls.

**SUPERSEDED by the pipeline reorder below.** This tradeoff is exactly what caused the bug that motivated the reorder: byte-based pre-selection ran before `DetectedStack` existed, so real-but-non-byte-detectable stack items (Next.js, Postgres) could get a repo excluded from top-K before manifest detection ever got a chance to prove it belonged. Confirmed via `var-raphael`: his `varsityline` and `vexaro`-frontend repos are genuinely Next.js projects, but got excluded from top-K before `DetectedStack` ran, so the LLM correctly (and confidently) reported "no evidence of Next.js" — technically true given what it was shown, but wrong given what's actually on his GitHub. The fix is not a patch, it's a full pipeline reorder — see next section.

## MAJOR PENDING WORK: Pipeline reorder (designed this session, not yet built)

This is the next thing to build. Full design agreed with the user, reasoning intact below — implement faithfully, don't just wing the shape from memory.

### The core problem being solved

Stack relevance was being decided **before** the system actually knew a repo's real stack. `selectTopK` picks repos using raw language bytes (cheap, no LLM call) specifically to avoid running `DetectedStack`'s LLM-backed manifest detection on every repo. But this means: a repo whose real stack relevance can only be proven via manifest data (Next.js via `package.json`'s `next` dependency, Postgres via `pgx`/`lib/pq` imports, Django via `requirements.txt`) can get excluded from consideration before that proof ever happens. The fix isn't a smarter guess at selection time — it's re-ordering so detection happens before relevance is judged at all.

### The new pipeline order, agreed and final

1. **Substance filter (stack-blind).** Same kind of thing `hasSubstance`/tree-file-count checks already do — just confirm a repo is real, non-trivial work. No stack matching, no language checks at this stage at all.
2. **Fraud/trust check (stack-blind).** Existing suspicious-padding detection (uniform commit timing), fork-vs-owned verification (`CommitsAhead`/`LinesChanged` thresholds), liveness checks on claimed URLs. Still no stack awareness needed here — this stage is purely "is this evidence trustworthy," independent of what job it's being evaluated against.
3. **Stack detection, now run on every repo that survived steps 1-2** (not a narrow pre-guessed shortlist). Sort survivors by substance/trust-quality first if a cap is still needed for cost control — the user confirmed the added LLM-call cost across more repos is acceptable ("it's a small token anyway"), so lean toward running detection broadly rather than aggressively pre-narrowing.
4. **Stack matching against the job**, using real `DetectedStack` data (not byte guesses). Drop any repo that doesn't hit at least one required job-stack item. This is the first point where "does this repo belong in this report" gets decided, and it's now based on ground truth.
5. **Survivors go to the LLM for scoring**, exactly as today, with one prompt addition (see "Repo dates" below).
6. **Per-stack-language percentage breakdown**, computed across all final surviving repos (see "Per-language breakdown" below) — this becomes much simpler to build correctly now that the repo set going into it has already been genuinely verified as stack-relevant, not a mix of real matches and guessed filler.

### Recency: removed as a mechanical score, kept as a narrative fact

Long discussion, final decision: **an old repo is not inherently worse evidence than a new one, if it's still substantive and trustworthy.** The current `ScoreRepo` formula's recency component (`MaxRecencyScore`, currently 25 of 100 points, decaying by 120-day half-life since `PushedAt`) get removed. Reasoning, in the user's own framing: a candidate who's shown strong Go usage across 3 years (2023-2026) across 4 repos is arguably a *stronger* signal than someone whose only Go evidence is one repo pushed last week — the current formula would score the fresher, thinner evidence higher, which is backwards.

**What replaces it:** nothing mechanical for multi-repo span — the LLM infers this itself from real per-repo dates already being shown to it. This requires adding a repo's **creation date**, not just `PushedAt` (last-push date), to the prompt — `CreatedAt` does not currently exist anywhere in `RawRepo`/`ScoredRepo`, needs to be fetched (standard GitHub repo API field, likely available in the same call that already fetches `PushedAt`, just not currently captured) and threaded through. With both dates present per repo, the LLM can naturally reason "used consistently 2023-2026" the same way it already synthesizes other multi-fact observations — no separate span-computation code needed, this was explicitly simplified out during design once the user pointed out the dates alone are sufficient raw material.

**What's kept, with reduced/different mechanical weight:** *current* engagement — i.e., `commits90d`/`activeWeeks90d`, "is this person still actively building right now" — is a genuinely different question from "how long ago was this repo last touched," and the user explicitly wants this to keep some real mechanical score weight, separate from the old last-push-date decay. The old `MaxRecencyScore` points need to be redistributed across the remaining components (stack match, current-activity, tree quality, liveness) once the formula is rebuilt — exact redistribution not yet decided, needs fresh design work, don't assume equal split.

### Per-language stack percentage breakdown (still not built, but now sequenced correctly)

From an earlier session: user wants an absolute (not relative-to-candidate) percentage per job-stack language, e.g. "Go 100% across 3 repos, Python 60% across 2 repos" — lets a recruiter filter/prioritize by a specific language even when the candidate's blended overall score is mediocre due to some other required language they're weak in. Decided absolute over relative specifically so the same percentage means the same thing across different candidates (needed for fair filtering/comparison).

This is now sequenced *after* the pipeline reorder above, deliberately — computing this cleanly depends on already having a correctly-verified, non-filler set of surviving repos per language. Formula still not designed — likely reuses per-repo signal types similar to `ScoreRepo` (tree quality, liveness, current-activity) but scoped per-language rather than per-repo, weighting each language's contribution within a repo by that language's byte-percentage inside it. Needs real design work before code, same as before — don't invent a formula in the moment.

### Files this reorder will touch, once built

- `internal/ranking/topk.go` — `selectTopK`, `BuildTopRepos`, the whole selection-order needs restructuring around the new stage order.
- `internal/ranking/score.go` — remove `MaxRecencyScore`/recency-decay math, add `CreatedAt` to `ScoredRepo`, redesign point redistribution.
- `internal/extractor/github/repos.go` — fetch and populate `CreatedAt` on `RawRepo`.
- `internal/scoring/finalscore.go` — formula changes following score.go's redistribution.
- `internal/scoring/detectstack.go` — likely needs to run against a wider set of repos than before (post substance+fraud filter, pre-stack-match), not the narrow shortlist it may currently assume.
- `internal/llm/prompt.go` — add `CreatedAt` alongside the existing `PushedAt` line in the per-repo prompt block.

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
