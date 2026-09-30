# Groundtruth: Agent Handoff

Read this first in any new session. It replaces the old AGENTS.md, which was outdated and deleted. If anything here conflicts with the code, the code wins, but tell the owner.

## Working rules

1. Never add comments to any code you write or edit. Existing comments in files you did not author may be left alone unless asked. This rule applies to Go, SQL, and every other language.
2. Output every file you create or modify as an artifact, using the `/mnt/user-data/outputs` directory and `present_files`. Do not paste finished files inline.
3. Do not read or echo `.env`. It holds secrets. Exclude it when unzipping.
4. The owner does not want endless polishing. Scoring is a recruiter aid, not an oracle, and the final decision is always the recruiter's. Prompt wording is a soft lever: the model sometimes ignores individual rules, and that is accepted as minor. Do not chase it further without being asked.
5. Keep responses short. The owner is often on a phone. Ask at most one or two questions at a time. When the owner says "no code yet", discuss only.
6. Go is not installed in the sandbox, so code cannot be compiled or run here. Say so plainly, verify edits by reading the changed regions and diffing against the original, and ask the owner to run `go build ./...`.
7. Anything that gates by plan must go through the central plan config described below. Never hardcode a limit number in a handler or query.

## What this is

Groundtruth ranks job applicants by evidence from their GitHub profiles instead of their resumes. Recruiters create a job (title, description, stack), candidates apply with a GitHub username, and a pipeline extracts GitHub evidence, detects each repo's real tech stack, and has an LLM (Mistral) judge fit. Every score traces back to repos and contributions a recruiter can open and check.

Module: `github.com/var-raphael/groundtruth`. Go 1.26, Postgres via pgx, `go-github` v66, no web framework beyond `net/http`.

## Layout

- `cmd/api`: the HTTP server entry point. `cmd/stackdebug` is a debug tool. `cmd/promptdebug` was deleted and must stay deleted.
- `internal/api/handlers`: HTTP handlers (jobs, apply, candidates, outreach, scan).
- `internal/api/middleware`: CORS and `FakeAuth`. `FakeAuth` authenticates every request as a dev recruiter with ID `00000000-0000-0000-0000-000000000001`. Real auth does not exist yet.
- `internal/db/queries`: all SQL. `internal/db/migrations`: numbered `.sql` files (001, 002, 003, 006, 007, 008 exist; 004 and 005 are absent).
- `internal/extractor/github`: repo fetching, activity stats, contributions, liveness checks.
- `internal/ranking`: repo filtering, per-repo scoring, top-K selection.
- `internal/scoring`: stack detection, LLM scoring pipeline, final score math.
- `internal/llm`: Mistral client, prompts, schemas.
- `internal/worker`: `Scan` (the extract-then-score pipeline) and the scheduler.
- `internal/outreach`: outreach email drafting.
- `pkg/config`: env loading.

## Pipeline

Candidate statuses: `queued` to `extracting` to `extracted` to `scoring` to `scored`, or `failed`.

1. Extraction pulls owned repos, filters forks and thin repos, computes activity stats, contributions to other projects, and liveness of claimed deployments. The evidence snapshot is saved to `candidate_evidence`.
2. Stack detection: a shortlist of repos gets an LLM-assisted manifest and infra file lookup, so a repo's stack comes from real dependency files, not just language bytes.
3. If no repo verifies against the job's stack but repos exist, `ranking.FallbackRepos` supplies up to 4 repos and sets `GithubEvidence.StackUnmatched`. This flag is never persisted. It changes the prompt and sets the warning "repos were found but none match the job's required stack".
4. The LLM returns a judgment score and positive and negative reasons with evidence links. The final score is computed in code.

Final score weights (in `scoring/finalscore.go`, must sum to 1.0): stack match 0.25, evidence strength 0.25, contributions 0.15, LLM judgment 0.35. Evidence strength is the average non-stack repo quality across the scored repos and is deliberately independent of stack match. The overall score means job fit. The four component scores are returned separately so a UI can sort by any of them.

Contribution scoring (`contributionsScore` in `finalscore.go`) is rank-then-cap, not a binary floor: a contribution only qualifies if the project has 20+ contributors and 100+ stars (`contributionMinContributors`, `contributionMinStars`); qualifying ones are ranked by a credibility score blending merged PR count, contributor count, and stars, all log-scaled (`contributionCredibility`); only the best 5 (`contributionMaxRanked`) are used; the final score is 35% quantity (count up to 5, capped) plus 65% average credibility of the best 5 (`contributionQuantityWeight`). This replaced an older formula that gave a flat floor of 6 to any single qualifying contribution and a hard 10 once 3 contributions cleared a threshold, which clustered unrelated candidates at the same score and let many weak contributions outscore few strong ones. Verified against the test job: var-raphael 3.8 (1 strong contribution), sindresorhus 8.3 and tj 7.8 (5 strong each, correctly no longer tied at 10), olly/aryagohar/secnot still 0.

`worker.Scan` takes `ScanOptions{Force, CandidateID, JobID}`. Without `Force`, saved evidence is reused and only scoring re-runs. The scheduler ticks every minute, picks up queued and extracted candidates, and resets candidates stuck in `scoring` for over 10 minutes.

Config: `godotenv.Overload()` is used so `.env` wins over shell variables. `MISTRAL_API_KEY` takes comma-separated keys and one LLM worker runs per key. Two keys on separate accounts double throughput.

## Routes

- `POST /jobs`, `GET /jobs`, `GET /jobs/{id}`, `DELETE /jobs/{id}`
- `POST /jobs/{id}/apply`
- `GET /jobs/{id}/candidates`, `GET /jobs/{id}/reports`
- `GET /candidates/{id}`, `GET /candidates/{id}/report`
- `GET /candidates/{id}/outreach` (read saved draft), `POST /candidates/{id}/outreach` (generate; `?regenerate=true` to replace), `PUT /candidates/{id}/outreach` (save an edit)
- `POST /scan?candidate_id=<id>` (the only rescan path: ownership check, plan check, usage recorded on success; `force=true` re-extracts). `POST /scan` with no candidate_id is the scheduler's queued-candidate pickup and is unlimited.
- `GET /health`

Reports are paginated, default 20, max 100.

## Done in the last session

- Job-scoped rescan endpoint (later removed, see below).
- Stack-mismatch fallback so off-stack candidates get real reasoning instead of a blank zero.
- Removed a false "no valid paths" error in `llm/rankprompt.go` that fired whenever the model correctly found no manifests.
- Evidence strength decoupled from stack match.
- Contribution weight raised from 0.10 to 0.15, LLM weight lowered from 0.40 to 0.35.
- Prompt changes in `llm/prompt.go`: evidence links must be supported by that repo's own data, no penalty for technologies living in separate repos, no negatives that concede themselves, and a job-relative benchmark for off-stack candidates.
- `.env` now overrides shell variables.
- Activity stats retry wait raised from 2s to 5s.

Reference job for testing: "Full Stack Engineer", stack Next.js, Go, Postgres. Test candidates and results at last run: var-raphael 8.2, sindresorhus 5.7, tj 5.1, olly-techie 3.3, aryagohar 2.6, secnot 1.4. All six scored.

## Done in the plan and limits session

- Central plan config: `internal/plans/plans.go` (free, pro, internal). Every limit lives there. `plans.Exceeds` handles the `Unlimited = -1` sentinel. The old hardcoded `planCandidateLimits` map in `handlers/jobs.go` is gone; new jobs get 20 (free) or 100 (pro) candidates.
- Usage log: migration `009_usage_events.sql`, queries in `db/queries/usage.go`, and `UsageStore` in `usage_adapter.go`. `plans.Check` in `plans/limits.go` decides allowed or denied (throttle, then daily limit, then lifetime limit) and returns a reason, a retry time, and a plain message. Only successful actions are recorded.
- Outreach persistence (task #1): migration `010_outreach_drafts.sql`, `models/outreach.go`, `queries/outreach_drafts.go`, and the three handlers in `handlers/outreach.go`. Drafts carry `edited` and `stale` flags. Editing costs nothing. A denied action returns 429 (throttled or daily) or 403 (lifetime) with `reason` and `retryAfterSeconds`. Verified end to end.
- Rescan limits (task #2): job-wide rescan was removed because per-candidate rescan covers every real need and the job-wide one was the most expensive action in the product. `POST /scan?candidate_id=` now checks ownership, refuses if the candidate is mid-scan, and runs the plan check. Usage is recorded only when the candidate ends `scored`.
- Job edit and delete-with-download: see the Job immutability section above.
- Export (task #4): see the roadmap entry below.
- The dev recruiter's plan is set with `UPDATE recruiters SET plan = 'free' WHERE id = '00000000-0000-0000-0000-000000000001'`. Switch it to `pro` or `internal` to test other tiers.

## Job immutability (done and verified)

A job is editable only while it has zero candidates, and locked from the first application on. `PUT /jobs/{id}` rejects with 409 once the job has one or more candidates; otherwise it updates and returns the job.

Delete is two-step, in `handlers/jobs.go` DeleteJob plus `db/queries/job_delete_confirmations.go` and migration `011_job_delete_confirmations.sql`. A job with zero candidates deletes immediately (204). A job with candidates returns 409 with a `confirmToken` and `candidateCount`, deletes nothing. Calling `GET /jobs/{id}/export` (any format) marks that job's pending confirmation as downloaded. Calling delete again with `?confirm_token=` then succeeds (204) if the report was downloaded, or 412 otherwise. Tokens expire after 1 hour. Verified end to end: 409 with candidates, 412 before export, export, 204 after, job confirmed gone with a 404.

Known side effect, accepted, not fixed: a candidate created moments before a delete can be mid-scan when the delete lands. The scheduler's background scoring goroutine then tries to save a report for a candidate_id that no longer exists, fails on the `candidate_reports` foreign key, and logs an error. No data corruption, nothing orphaned, the constraint does its job. Owner wants this left as a traceable log line rather than fixed.

## Roadmap, agreed and in order

Build order: #1, #2, #4, #3, #5. Auth comes after or alongside, because free versus paid needs a real recruiter identity. Only #3 and #5 remain; #1, #2, #4, and the plan config are done and verified.

### Central plan config (build first, used by everything)

One file, for example `internal/plans/plans.go` or a JSON file loaded at startup, holds all limits per plan, read through a single function. No limit number may appear anywhere else. Unlimited is one sentinel value the checking function understands. The owner's intended shape:

```
free:  candidates 20, jobs 1, outreach candidates 3, ...
pro:   price 59 USD, candidates 100, jobs 4, ...
```

Keys to define: `price_usd`, `max_jobs`, `max_candidates_per_job`, `outreach_candidates_lifetime`, `outreach_generations_per_candidate_per_day`, `rescan_candidates_lifetime`, `rescan_per_candidate_per_day`, `min_interval_seconds`, `export_formats`, `public_link_outreach_limit`. Start as a file. Move to a database table only if limits must change without a redeploy.

Add a separate unlimited internal plan for the owner's own dashboard, so marketing jobs are not subject to the 20-candidate and 1-job caps.

Track usage in one append-only usage log: who did what to which candidate and when. Every limit is a query over that log. Only successful actions count. Every denial should return a specific reason and a retry time: throttled, daily limit reached, or lifetime cap reached.

### Limit rules (final)

| | Free | Pro |
|---|---|---|
| Jobs | 1 | 4 |
| Candidates per job | 20 | 100 |
| Outreach | 3 distinct candidates ever; each generated up to 3 times per rolling 24h; first generation counts as one of the 3 | unlimited candidates, throttled |
| Rescan | 3 distinct candidates ever; each once per rolling 24h | any candidate, up to 3 times per rolling 24h |
| Throttle | 10 minutes | 1 minute |

- The throttle is per candidate and shared across rescan and outreach.
- Windows are rolling 24 hours.
- Editing and saving an outreach draft costs nothing. Only generate and regenerate count.
- A rescan of an already-counted candidate does not use a new lifetime slot.
- Failed rescans do not burn allowance.

### #1 Outreach persistence (done)

### #2 Rescan limits (done; the two remaining checks, daily_limit and the 4th-candidate lifetime_limit, were expected to behave like outreach)

### #4 Export (done)

`GET /jobs/{id}/export?format=json|csv|excel|pdf` (default json). Gated by plan (`plans.AllowsExport`): free gets json and csv, pro and internal get all four. One endpoint in `handlers/export.go`, builders in `internal/export/export.go`, an unpaginated `queries.AllReportsByJob` added for this. Each format includes, per candidate: the summary (all four scores, stack match, status, warning), repos with detected stack and activity, external contributions, and stack coverage percentages, plus the existing positive/negative reasoning. CSV is one file with five sections. Excel is five sheet tabs (Candidates, Repos, Contributions, Stack Coverage, Reasoning). PDF flows one section per candidate with page breaks; uses `pdf.UnicodeTranslatorFromDescriptor("")` to render em dashes and curly quotes correctly, since gofpdf's core Helvetica font mangles raw UTF-8 without it. Two new dependencies (`github.com/xuri/excelize/v2`, `github.com/jung-kurt/gofpdf`) are in go.mod; run `go mod tidy` once to fetch them and fix go.sum, since Claude cannot run go get in its sandbox. Not yet restricted from public view-only links, because those do not exist yet (#3).

### #3 Public job creation (no user auth) — in progress

Step 1 done: share link infrastructure. Migration `012_job_share_links.sql` (`job_share_links` table: token, job_id, created_at, revoked_at). Queries in `db/queries/job_share_links.go`. Handler `handlers/share_links.go`, owner-only via FakeAuth: `POST /jobs/{id}/share-link` (creates, or returns the existing active one if already created — idempotent), `GET /jobs/{id}/share-link` (fetch active link, 404 if none), `DELETE /jobs/{id}/share-link/{token}` (revoke, sets revoked_at, 404 if not found or already revoked). Not yet done: the public-facing endpoint that actually consumes a token to create a job with candidates, the dashboard-secret protection, the 20-candidate cap, and the public outreach-of-3 limit.

Next up in this task: the public job-creation endpoint itself.

An endpoint to create a job and add candidates from an array (GitHub profile and LinkedIn URL). Protection is the owner's dashboard secret: the backend accepts the request only when it comes from the owner's site (`groundtruth.vercel.app`) with the dashboard password. Keep the secret in server-side code, since a key in browser code is visible to anyone. LinkedIn is stored for display only and is not scored. Maximum 20 candidates, enforced server-side. Share links use a random unguessable token, never the job ID, do not expire, and are revoked by deleting them from the dashboard. A public link allows drafting up to 3 outreach emails, counted server-side against the token in the same usage log. No export on public links.

### #5 Landing page

The live tester is removed. Instead, a public pre-scored demo page shows several candidates (the owner's friends' profiles plus one public figure) scored against one demo job. Zero per-visitor cost. The demo is refreshed deliberately by the owner, not automatically.

### Signup behavior

When someone signs up from a shared view-only link, they start with a blank account and may copy the demo job as a template (title, description, and stack only, no candidates). Do not transfer candidates: those are real people's profiles.
