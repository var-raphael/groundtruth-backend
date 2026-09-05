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

func hasSubstance(tree *ghextractor.TreeSummary) bool {
	return tree != nil && len(tree.Paths) >= minTreeFilesForSubstance
}

func BuildTopRepos(ctx context.Context, client *github.Client, username string, allRepos []ghextractor.RawRepo, jobStack []string, topK int) ([]ScoredRepo, error) {
	if topK <= 0 {
		topK = defaultTopK
	}

	kept, err := FilterOwnedRepos(ctx, client, username, allRepos)
	if err != nil {
		return nil, err
	}

	scored := make([]ScoredRepo, 0, len(kept))
	for _, repo := range kept {
		activity, actErr := ghextractor.FetchActivity(ctx, client, username, repo.Name)
		if actErr != nil {
			activity = nil
		}

		tree, treeErr := ghextractor.FetchTree(ctx, client, username, repo.Name, repo.DefaultBranch)
		if treeErr != nil {
			tree = nil
		}

		if !hasSubstance(tree) {
			continue
		}

		languages, langErr := ghextractor.FetchLanguages(ctx, client, username, repo.Name)
		if langErr != nil {
			languages = nil
		}

		sr := ScoreRepo(repo, activity, tree, languages, jobStack)
		if actErr != nil {
			sr.ActivityError = actErr.Error()
		}
		if treeErr != nil {
			sr.TreeError = treeErr.Error()
		}
		if langErr != nil {
			sr.LanguagesError = langErr.Error()
		}

		scored = append(scored, sr)
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > topK {
		scored = selectTopK(scored, jobStack, topK)
	}

	var wg sync.WaitGroup
	for i := range scored {
		url := scored[i].Repo.HomepageURL
		if strings.TrimSpace(url) == "" {
			continue
		}
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			check := ghextractor.CheckLiveness(ctx, url)
			scored[i].Liveness = &check
			scored[i].Score = scored[i].Score - firstPassLivenessCredit(url) + livenessScore(url, &check)
		}(i, url)
	}
	wg.Wait()

	for i := range scored {
		timings, err := ghextractor.FetchRecentCommitTimings(ctx, client, username, scored[i].Repo.Name, 0)
		if err != nil {
			scored[i].CommitsError = err.Error()
			continue
		}
		scored[i].Commits = timings
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored, nil
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

	if len(selected) < topK {
		for i := range scored {
			if len(selected) >= topK {
				break
			}
			if selected[i] {
				continue
			}
			if !hasRealSubstanceForFiller(scored[i]) {
				continue
			}
			selected[i] = true
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

const minTreeFilesForFiller = 8

func hasRealSubstanceForFiller(sr ScoredRepo) bool {
	if sr.Tree == nil || len(sr.Tree.Paths) < minTreeFilesForFiller {
		return false
	}
	if len(sr.Languages) < 2 {
		return false
	}
	return true
}

func repoCoversLanguage(sr ScoredRepo, lang string) bool {
	if len(sr.Languages) == 0 {
		return false
	}
	var totalBytes int
	for _, bytes := range sr.Languages {
		totalBytes += bytes
	}
	if totalBytes == 0 {
		return false
	}
	const presenceFloor = 0.03
	for reportedLang, bytes := range sr.Languages {
		if !strings.EqualFold(reportedLang, lang) {
			continue
		}
		proportion := float64(bytes) / float64(totalBytes)
		return proportion >= presenceFloor
	}
	return false
}

func firstPassLivenessCredit(url string) float64 {
	return livenessScore(url, nil)
}
