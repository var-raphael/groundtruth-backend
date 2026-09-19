package ranking

import (
	"context"
	"sort"
	"strings"
	"sync"

	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/google/go-github/v66/github"
)

const defaultTopK = 6

const minTreeFilesForSubstance = 5

const maxConcurrentGithubCalls = 10

func runBounded(n int, fn func(i int)) {
	sem := make(chan struct{}, maxConcurrentGithubCalls)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func hasSubstance(tree *ghextractor.TreeSummary) bool {
	return tree != nil && len(tree.Paths) >= minTreeFilesForSubstance
}

func BuildTopRepos(ctx context.Context, client *github.Client, username string, allRepos []ghextractor.RawRepo, jobStack []string, topK int) ([]ScoredRepo, error) {
	kept, err := FilterOwnedRepos(ctx, client, username, allRepos)
	if err != nil {
		return nil, err
	}

	prelim := make([]ScoredRepo, len(kept))
	runBounded(len(kept), func(i int) {
		repo := kept[i]
		tree, treeErr := ghextractor.FetchTree(ctx, client, username, repo.Name, repo.DefaultBranch)
		if treeErr != nil {
			tree = nil
		}

		var languages ghextractor.LanguageBreakdown
		var langErr error
		if hasSubstance(tree) {
			languages, langErr = ghextractor.FetchLanguages(ctx, client, username, repo.Name)
			if langErr != nil {
				languages = nil
			}
		}

		sr := ScoreRepo(repo, nil, tree, languages, jobStack)
		if treeErr != nil {
			sr.TreeError = treeErr.Error()
		}
		if langErr != nil {
			sr.LanguagesError = langErr.Error()
		}
		prelim[i] = sr
	})

	scored := make([]ScoredRepo, 0, len(prelim))
	for _, sr := range prelim {
		if !hasSubstance(sr.Tree) {
			continue
		}
		scored = append(scored, sr)
	}

	runBounded(len(scored), func(i int) {
		activity, actErr := ghextractor.FetchActivity(ctx, client, username, scored[i].Repo.Name)
		if actErr != nil {
			scored[i].ActivityError = actErr.Error()
			return
		}
		scored[i] = ScoreRepo(scored[i].Repo, activity, scored[i].Tree, scored[i].Languages, jobStack)
	})

	livenessIndices := make([]int, 0, len(scored))
	for i := range scored {
		if strings.TrimSpace(scored[i].Repo.HomepageURL) != "" {
			livenessIndices = append(livenessIndices, i)
		}
	}
	runBounded(len(livenessIndices), func(j int) {
		i := livenessIndices[j]
		url := scored[i].Repo.HomepageURL
		check := ghextractor.CheckLiveness(ctx, url)
		scored[i].Liveness = &check
		scored[i].Score = scored[i].Score - firstPassLivenessCredit(url) + livenessScore(url, &check)
	})

	runBounded(len(scored), func(i int) {
		timings, err := ghextractor.FetchRecentCommitTimings(ctx, client, username, scored[i].Repo.Name, 0)
		if err != nil {
			scored[i].CommitsError = err.Error()
			return
		}
		scored[i].Commits = timings
	})

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored, nil
}

func SelectVerifiedTopRepos(scored []ScoredRepo, jobStack []string, topK int) []ScoredRepo {
	if topK <= 0 {
		topK = defaultTopK
	}
	if len(scored) <= topK {
		var verified []ScoredRepo
		for _, sr := range scored {
			if sr.StackMatchScore > 0 {
				verified = append(verified, sr)
			}
		}
		return verified
	}
	return selectTopK(scored, jobStack, topK)
}

func selectTopK(scored []ScoredRepo, jobStack []string, topK int) []ScoredRepo {
	selected := make(map[int]bool, topK)

	for _, lang := range jobStack {
		best := -1
		for i := range scored {
			if selected[i] {
				continue
			}
			if !repoCoversLanguage(scored[i], lang) {
				continue
			}
			if best == -1 || scored[i].Score > scored[best].Score {
				best = i
			}
		}
		if best != -1 {
			selected[best] = true
			if len(selected) >= topK {
				break
			}
		}
	}

	for len(selected) < topK {
		pickedThisRound := false
		for _, lang := range jobStack {
			if len(selected) >= topK {
				break
			}
			best := -1
			for i := range scored {
				if selected[i] {
					continue
				}
				if !repoCoversLanguage(scored[i], lang) {
					continue
				}
				if best == -1 || scored[i].Score > scored[best].Score {
					best = i
				}
			}
			if best != -1 {
				selected[best] = true
				pickedThisRound = true
			}
		}
		if !pickedThisRound {
			break
		}
	}

	if len(selected) < topK {
		type candidate struct {
			index int
			score float64
		}
		var remaining []candidate
		for i := range scored {
			if selected[i] {
				continue
			}
			if scored[i].StackMatchScore <= 0 {
				continue
			}
			remaining = append(remaining, candidate{index: i, score: scored[i].Score})
		}
		sort.Slice(remaining, func(i, j int) bool {
			return remaining[i].score > remaining[j].score
		})
		for _, c := range remaining {
			if len(selected) >= topK {
				break
			}
			selected[c.index] = true
		}
	}

	out := make([]ScoredRepo, 0, len(selected))
	for i := range scored {
		if selected[i] {
			out = append(out, scored[i])
		}
	}
	return out
}

func repoCoversLanguage(sr ScoredRepo, lang string) bool {
	for _, have := range sr.DetectedStack {
		if StackNamesMatch(have, lang) {
			return true
		}
	}
	return false
}

func firstPassLivenessCredit(url string) float64 {
	return livenessScore(url, nil)
}
