package results

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type ResultStatus string

const (
	StatusSucceeded     ResultStatus = "succeeded"
	StatusFailed        ResultStatus = "failed"
	StatusBlocked       ResultStatus = "blocked"
	StatusHumanRequired ResultStatus = "human_required"
	StatusRetryPending  ResultStatus = "retry_pending"
)

func (s ResultStatus) Valid() bool {
	switch s {
	case StatusSucceeded, StatusFailed, StatusBlocked, StatusHumanRequired, StatusRetryPending:
		return true
	default:
		return false
	}
}

type AllowedRepos struct {
	Writable []string `json:"writable"`
	Readable []string `json:"readable"`
}

type NextTaskIntent struct {
	TaskID             string         `json:"task_id,omitempty"`
	DedupeKey          string         `json:"dedupe_key,omitempty"`
	Type               string         `json:"type,omitempty"`
	Role               string         `json:"role"`
	SourceRepo         string         `json:"source_repo,omitempty"`
	TargetRepo         string         `json:"target_repo,omitempty"`
	Objective          string         `json:"objective"`
	AcceptanceCriteria string         `json:"acceptance_criteria,omitempty"`
	DependencyRelation string         `json:"dependency_relation,omitempty"`
	AllowedRepos       AllowedRepos   `json:"allowed_repos,omitempty"`
	SuccessCriteria    []string       `json:"success_criteria,omitempty"`
	Context            map[string]any `json:"context,omitempty"`
}

type CodeHostWritebackIntent struct {
	DedupeKey          string   `json:"dedupe_key,omitempty"`
	Kind               string   `json:"kind"`
	ProjectID          string   `json:"project_id"`
	Title              string   `json:"title,omitempty"`
	Description        string   `json:"description,omitempty"`
	SourceBranch       string   `json:"source_branch,omitempty"`
	TargetBranch       string   `json:"target_branch,omitempty"`
	RemoveSourceBranch bool     `json:"remove_source_branch,omitempty"`
	IssueIID           int      `json:"issue_iid,omitempty"`
	MergeRequestIID    int      `json:"merge_request_iid,omitempty"`
	Body               string   `json:"body,omitempty"`
	Labels             []string `json:"labels,omitempty"`
}

type EffectIntent struct {
	DedupeKey string         `json:"dedupe_key,omitempty"`
	Kind      string         `json:"kind"`
	Target    string         `json:"target"`
	Critical  bool           `json:"critical,omitempty"`
	Request   map[string]any `json:"request,omitempty"`
}

type CanonicalResult struct {
	TaskID        string                    `json:"task_id" binding:"required"`
	RunID         string                    `json:"run_id" binding:"required"`
	Status        ResultStatus              `json:"status" binding:"required"`
	Summary       string                    `json:"summary"`
	Artifacts     map[string]any            `json:"artifacts,omitempty"`
	BlockerReason *string                   `json:"blocker_reason,omitempty"`
	NextTasks     []NextTaskIntent          `json:"next_tasks,omitempty"`
	Writebacks    []CodeHostWritebackIntent `json:"writebacks,omitempty"`
	Effects       []EffectIntent            `json:"effects,omitempty"`
}

func Canonicalize(result CanonicalResult) ([]byte, string, error) {
	if result.TaskID == "" {
		return nil, "", errors.New("result task_id is required")
	}
	if result.RunID == "" {
		return nil, "", errors.New("result run_id is required")
	}
	if !result.Status.Valid() {
		return nil, "", fmt.Errorf("invalid result status %q", result.Status)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, "", fmt.Errorf("marshal canonical result: %w", err)
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

func DecodeCanonicalStrict(raw json.RawMessage) (CanonicalResult, error) {
	result, err := decodeStrict[CanonicalResult](raw)
	if err != nil {
		return CanonicalResult{}, err
	}
	if _, _, err := Canonicalize(*result); err != nil {
		return CanonicalResult{}, err
	}
	return *result, nil
}
