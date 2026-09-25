package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/results"
)

const (
	defaultNextTaskType       = "generated_from_next_task"
	defaultDependencyRelation = "upstream_fix"
)

func BuildNextTaskExpansion(parent *model.Task, parentRun *model.Run, next results.NextTaskIntent, index int) (*model.Task, *model.Dependency, error) {
	if parent == nil {
		return nil, nil, errors.New("parent task is required")
	}
	if parentRun == nil {
		return nil, nil, errors.New("parent run is required")
	}

	role := strings.TrimSpace(next.Role)
	if role == "" {
		return nil, nil, errors.New("next task role is required")
	}
	objective := strings.TrimSpace(next.Objective)
	if objective == "" {
		return nil, nil, errors.New("next task objective is required")
	}

	taskID := strings.TrimSpace(next.TaskID)
	dedupeKey := strings.TrimSpace(next.DedupeKey)
	if dedupeKey == "" {
		dedupeKey = fmt.Sprintf("next:%s:%d", parent.TaskID, index)
	}
	taskType := strings.TrimSpace(next.Type)
	if taskType == "" {
		taskType = defaultNextTaskType
	}
	acceptanceCriteria := strings.TrimSpace(next.AcceptanceCriteria)
	if acceptanceCriteria == "" && len(next.SuccessCriteria) > 0 {
		acceptanceCriteria = strings.Join(next.SuccessCriteria, "\n")
	}

	contextJSON, err := buildNextTaskContextJSON(parent, parentRun, next, index)
	if err != nil {
		return nil, nil, err
	}

	child := &model.Task{
		TaskID:             taskID,
		DedupeKey:          dedupeKey,
		Type:               taskType,
		Role:               role,
		RepositoryID:       parent.RepositoryID,
		SourceRepo:         strings.TrimSpace(next.SourceRepo),
		TargetRepo:         strings.TrimSpace(next.TargetRepo),
		Objective:          objective,
		AcceptanceCriteria: acceptanceCriteria,
		Status:             model.TaskStatusPending,
		ContextJSON:        contextJSON,
	}

	relation := strings.TrimSpace(next.DependencyRelation)
	if relation == "" {
		relation = defaultDependencyRelation
	}
	metadata := map[string]any{
		"parent_task_id": parent.TaskID,
		"parent_run_id":  parentRun.RunID,
		"index":          index,
		"next_task":      next,
	}
	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal dependency metadata: %w", err)
	}

	dependency := &model.Dependency{
		ParentTaskID: parent.TaskID,
		ChildTaskID:  taskID,
		Relation:     relation,
		Status:       model.DependencyStatusPending,
		MetadataJSON: string(rawMetadata),
	}

	return child, dependency, nil
}

func (o *Orchestrator) expandResultNextTasks(ctx context.Context, parent *model.Task, parentRun *model.Run, payload results.CanonicalResult) (int, error) {
	if len(payload.NextTasks) == 0 {
		return 0, nil
	}
	// next_tasks 是对结果应用的增量丰富:依赖端口未装配时跳过扩展而不
	// 阻断结果本身(审查放行等常规结果现也携带派生意图)。生产装配恒带
	// 端口;测试与最小装配只损失派生任务,不损失主结果。
	if o.dependencies == nil {
		return 0, nil
	}

	expanded := 0
	for i, next := range payload.NextTasks {
		child, dependency, err := BuildNextTaskExpansion(parent, parentRun, next, i)
		if err != nil {
			return 0, err
		}

		createdChild, err := o.CreateTask(ctx, child)
		if err != nil {
			return 0, fmt.Errorf("create next task %d: %w", i, err)
		}
		dependency.ChildTaskID = createdChild.TaskID
		if err := o.dependencies.UpsertByIdentity(ctx, dependency); err != nil {
			return 0, fmt.Errorf("upsert dependency for next task %s: %w", createdChild.TaskID, err)
		}
		expanded++
	}

	return expanded, nil
}

func (o *Orchestrator) resolveDependenciesForResult(ctx context.Context, child *model.Task, payload results.CanonicalResult) error {
	if o.dependencies == nil {
		return nil
	}

	dependencies, err := o.dependencies.ListByChildTaskID(ctx, child.TaskID)
	if err != nil {
		return fmt.Errorf("list dependencies for child task %s: %w", child.TaskID, err)
	}
	if len(dependencies) == 0 {
		return nil
	}

	switch payload.Status {
	case results.StatusSucceeded:
		return o.resolveSuccessfulChildDependencies(ctx, dependencies)
	case results.StatusFailed,
		results.StatusRetryPending,
		results.StatusBlocked,
		results.StatusHumanRequired:
		return o.blockChildDependencies(ctx, dependencies)
	default:
		return nil
	}
}

func (o *Orchestrator) resolveSuccessfulChildDependencies(ctx context.Context, dependencies []model.Dependency) error {
	for i := range dependencies {
		dependencies[i].Status = model.DependencyStatusResolved
		if err := o.dependencies.Update(ctx, &dependencies[i]); err != nil {
			return fmt.Errorf("resolve dependency %s: %w", dependencies[i].DependencyID, err)
		}

		parent, err := o.tasks.GetByTaskID(ctx, dependencies[i].ParentTaskID)
		if err != nil {
			return fmt.Errorf("get dependency parent task %s: %w", dependencies[i].ParentTaskID, err)
		}
		if parent.Status != model.TaskStatusWaitingUpstream {
			continue
		}
		allResolved, err := o.allParentDependenciesResolved(ctx, parent.TaskID)
		if err != nil {
			return err
		}
		if !allResolved {
			continue
		}

		previous := parent.Status
		if err := o.stateMachine.Transition(parent, model.TaskStatusPending); err != nil {
			return err
		}
		o.policy.Apply(parent, previous, model.TaskStatusPending, o.now())
		if err := o.tasks.Update(ctx, parent); err != nil {
			return fmt.Errorf("restore dependency parent task %s: %w", parent.TaskID, err)
		}
	}
	return nil
}

func (o *Orchestrator) allParentDependenciesResolved(ctx context.Context, parentTaskID string) (bool, error) {
	dependencies, err := o.dependencies.ListByParentTaskID(ctx, parentTaskID)
	if err != nil {
		return false, fmt.Errorf("list dependencies for parent task %s: %w", parentTaskID, err)
	}
	if len(dependencies) == 0 {
		return false, nil
	}
	for _, dependency := range dependencies {
		if dependency.Status != model.DependencyStatusResolved {
			return false, nil
		}
	}
	return true, nil
}

func (o *Orchestrator) blockChildDependencies(ctx context.Context, dependencies []model.Dependency) error {
	for i := range dependencies {
		dependencies[i].Status = model.DependencyStatusBlocked
		if err := o.dependencies.Update(ctx, &dependencies[i]); err != nil {
			return fmt.Errorf("block dependency %s: %w", dependencies[i].DependencyID, err)
		}
	}
	return nil
}

func buildNextTaskContextJSON(parent *model.Task, parentRun *model.Run, next results.NextTaskIntent, index int) (string, error) {
	context := make(map[string]any, len(next.Context)+5)
	for key, value := range next.Context {
		context[key] = value
	}
	context["parent_task_id"] = parent.TaskID
	context["parent_run_id"] = parentRun.RunID
	context["next_task_index"] = index
	context["allowed_repos"] = next.AllowedRepos
	if parent.ContextJSON != "" {
		context["parent_context_json"] = parent.ContextJSON
	}

	rawContext, err := json.Marshal(context)
	if err != nil {
		return "", fmt.Errorf("marshal next task context: %w", err)
	}
	return string(rawContext), nil
}
