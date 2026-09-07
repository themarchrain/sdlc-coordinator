package agent

import "sdlc-coordinator/internal/results"

const WorkspaceStrategyCoordinatorPrepared = "coordinator_prepared"

type AllowedRepos = results.AllowedRepos

type RepoSpec struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Branch string `json:"branch,omitempty"`
	Access string `json:"access"`
}

type DeliverySpec struct {
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
}

type TaskPackage struct {
	TaskID            string         `json:"task_id"`
	RunID             string         `json:"run_id"`
	Type              string         `json:"type"`
	Role              string         `json:"role"`
	Objective         string         `json:"objective"`
	AllowedRepos      AllowedRepos   `json:"allowed_repos"`
	SuccessCriteria   []string       `json:"success_criteria"`
	WorkspaceStrategy string         `json:"workspace_strategy"`
	Repos             []RepoSpec     `json:"repos,omitempty"`
	Delivery          *DeliverySpec  `json:"delivery,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
}
