package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"sdlc-coordinator/internal/results"
)

func (o *Orchestrator) NormalizeRoleResult(ctx context.Context, contract results.RoleResultContract, raw json.RawMessage) (results.CanonicalResult, error) {
	if contract == nil {
		return results.CanonicalResult{}, errors.New("role result contract is required")
	}
	input, err := contract.DecodeStrict(raw)
	if err != nil {
		return results.CanonicalResult{}, fmt.Errorf("decode %s input: %w", contract.ToolName(), err)
	}
	taskID, runID := input.Identity()
	task, err := o.tasks.GetByTaskID(ctx, taskID)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	run, err := o.runs.GetByRunID(ctx, runID)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	if err := contract.Validate(task, run, input); err != nil {
		return results.CanonicalResult{}, err
	}
	canonical, err := contract.Normalize(task, run, input)
	if err != nil {
		return results.CanonicalResult{}, err
	}
	if _, _, err := results.Canonicalize(canonical); err != nil {
		return results.CanonicalResult{}, err
	}
	return canonical, nil
}
