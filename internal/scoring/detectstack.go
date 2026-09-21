package scoring

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"
	ghextractor "github.com/var-raphael/groundtruth/internal/extractor/github"
	"github.com/var-raphael/groundtruth/internal/llm"
	"github.com/var-raphael/groundtruth/internal/ranking"
)

const stackDetectionTemperature = 0.0
const maxReposPerStackDetectionBatch = 10

func ApplyDetectedStack(
	ctx context.Context,
	githubClient *github.Client,
	mistralClient *llm.Client,
	owner string,
	topRepos []ranking.ScoredRepo,
	jobStack []string,
) []ranking.ScoredRepo {
	out := make([]ranking.ScoredRepo, len(topRepos))
	copy(out, topRepos)

	for start := 0; start < len(out); start += maxReposPerStackDetectionBatch {
		end := start + maxReposPerStackDetectionBatch
		if end > len(out) {
			end = len(out)
		}
		applyDetectedStackToBatch(ctx, githubClient, mistralClient, owner, out[start:end], jobStack)
	}

	return out
}

func applyDetectedStackToBatch(
	ctx context.Context,
	githubClient *github.Client,
	mistralClient *llm.Client,
	owner string,
	batch []ranking.ScoredRepo,
	jobStack []string,
) {
	inputs := make([]llm.StackDetectionRepoInput, len(batch))
	for i := range batch {
		var manifestFiles []ghextractor.ManifestFile
		var infraFiles []ghextractor.InfraConfigFile
		if batch[i].Tree != nil {
			candidatePaths := ghextractor.FindCandidatePaths(batch[i].Tree.Paths)
			selection, err := llm.RankManifestAndInfraCandidates(ctx, mistralClient, candidatePaths)
			if err != nil {
				batch[i].DetectedStackError = err.Error()
			} else {
				files, err := ghextractor.FetchManifestFiles(ctx, githubClient, owner, batch[i].Repo.Name, selection.ManifestPaths)
				if err != nil {
					batch[i].DetectedStackError = err.Error()
				}
				manifestFiles = files

				iFiles, err := ghextractor.FetchInfraConfigFiles(ctx, githubClient, owner, batch[i].Repo.Name, selection.InfraPaths)
				if err != nil {
					if batch[i].DetectedStackError == "" {
						batch[i].DetectedStackError = err.Error()
					}
				}
				infraFiles = iFiles
			}
		}
		inputs[i] = llm.StackDetectionRepoInput{
			Name:             batch[i].Repo.Name,
			Languages:        batch[i].Languages,
			ManifestFiles:    manifestFiles,
			InfraConfigFiles: infraFiles,
		}
	}

	userPrompt := llm.BuildStackDetectionBatchPrompt(inputs)
	rawResponse, err := mistralClient.CompleteJSON(ctx, llm.StackDetectionBatchSystemPrompt(), userPrompt, stackDetectionTemperature)
	if err != nil {
		for i := range batch {
			batch[i].DetectedStackError = fmt.Sprintf("detecting stack batch for %s: %v", batch[i].Repo.Name, err)
		}
		return
	}

	detected, err := llm.ParseDetectedStackBatch(rawResponse)
	if err != nil {
		for i := range batch {
			batch[i].DetectedStackError = fmt.Sprintf("parsing detected stack batch for %s: %v", batch[i].Repo.Name, err)
		}
		return
	}

	byName := make(map[string][]string, len(detected.Repos))
	for _, entry := range detected.Repos {
		byName[entry.Name] = entry.Stack
	}

	for i := range batch {
		stack, ok := byName[batch[i].Repo.Name]
		if !ok {
			batch[i].DetectedStackError = fmt.Sprintf("no stack detection result returned for %s", batch[i].Repo.Name)
			continue
		}
		batch[i].DetectedStack = stack
		batch[i] = batch[i].RescoreWithDetectedStack(jobStack)
	}
}
