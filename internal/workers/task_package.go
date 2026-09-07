package workers

import (
	"encoding/json"
	"errors"
	"strings"

	"sdlc-coordinator/internal/agent"
	"sdlc-coordinator/internal/model"
)

func BuildRunnerTaskPackage(task *model.Task, run *model.Run) (agent.TaskPackage, error) {
	if task == nil {
		return agent.TaskPackage{}, errors.New("task is required")
	}
	if run == nil {
		return agent.TaskPackage{}, errors.New("run is required")
	}

	context := map[string]any{}
	if strings.TrimSpace(task.ContextJSON) != "" {
		if err := json.Unmarshal([]byte(task.ContextJSON), &context); err != nil {
			return agent.TaskPackage{}, err
		}
	}
	repos, err := runnerRepoSpecsFromContext(context)
	if err != nil {
		return agent.TaskPackage{}, err
	}
	delivery, err := runnerDeliverySpecForTask(task, repos)
	if err != nil {
		return agent.TaskPackage{}, err
	}

	return agent.TaskPackage{
		TaskID:            task.TaskID,
		RunID:             run.RunID,
		Type:              task.Type,
		Role:              task.Role,
		Objective:         task.Objective,
		AllowedRepos:      runnerAllowedRepos(task),
		SuccessCriteria:   splitRunnerCriteria(task.AcceptanceCriteria),
		WorkspaceStrategy: agent.WorkspaceStrategyCoordinatorPrepared,
		Repos:             repos,
		Delivery:          delivery,
		Context:           context,
	}, nil
}

func runnerRepoSpecsFromContext(context map[string]any) ([]agent.RepoSpec, error) {
	raw, ok := context["repos"]
	if !ok {
		raw, ok = context["repo_specs"]
	}
	if !ok || raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var repos []agent.RepoSpec
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func runnerDeliverySpecForTask(task *model.Task, repos []agent.RepoSpec) (*agent.DeliverySpec, error) {
	var writable []agent.RepoSpec
	for _, repo := range repos {
		if strings.EqualFold(strings.TrimSpace(repo.Access), "writable") {
			writable = append(writable, repo)
		}
	}
	switch len(writable) {
	case 0:
		return nil, nil
	case 1:
		baseBranch := strings.TrimSpace(writable[0].Branch)
		if baseBranch == "" {
			baseBranch = "main"
		}
		return &agent.DeliverySpec{
			Repo:       strings.TrimSpace(writable[0].Name),
			Branch:     model.DeliveryBranch(task),
			BaseBranch: baseBranch,
		}, nil
	default:
		return nil, errors.New("exactly one writable repository is supported per task")
	}
}

func runnerAllowedRepos(task *model.Task) agent.AllowedRepos {
	repos := agent.AllowedRepos{}
	if task.TargetRepo != "" {
		repos.Writable = append(repos.Writable, task.TargetRepo)
	}
	if task.TargetRepo == "" && task.SourceRepo != "" {
		repos.Writable = append(repos.Writable, task.SourceRepo)
	}
	if task.SourceRepo != "" && task.SourceRepo != task.TargetRepo {
		repos.Readable = append(repos.Readable, task.SourceRepo)
	}
	return repos
}

func splitRunnerCriteria(criteria string) []string {
	lines := strings.Split(criteria, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
