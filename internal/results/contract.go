package results

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"sdlc-coordinator/internal/model"
)

const (
	SubmitTesterResultTool    = "submit_tester_result"
	SubmitDeveloperResultTool = "submit_developer_result"
	SubmitReviewerResultTool  = "submit_reviewer_result"
)

// ToolNameForWorkerRole 返回 worker 角色对应的 MCP 结果提交工具名。
func ToolNameForWorkerRole(role string) (string, bool) {
	switch role {
	case "DeveloperWorker":
		return SubmitDeveloperResultTool, true
	case "ReviewerWorker":
		return SubmitReviewerResultTool, true
	case "TesterWorker":
		return SubmitTesterResultTool, true
	default:
		return "", false
	}
}

type RoleResultInput interface {
	Identity() (taskID string, runID string)
}

type RoleResultContract interface {
	ToolName() string
	Role() string
	InputSchema() map[string]any
	DecodeStrict(json.RawMessage) (RoleResultInput, error)
	Validate(task *model.Task, run *model.Run, input RoleResultInput) error
	Normalize(task *model.Task, run *model.Run, input RoleResultInput) (CanonicalResult, error)
}

type ContractRegistry struct {
	contracts map[string]RoleResultContract
}

func NewDefaultContractRegistry() *ContractRegistry {
	return NewContractRegistry(NewTesterContract(), NewDeveloperContract(), NewReviewerContract())
}

func NewContractRegistry(contracts ...RoleResultContract) *ContractRegistry {
	registry := &ContractRegistry{contracts: make(map[string]RoleResultContract, len(contracts))}
	for _, contract := range contracts {
		if contract != nil {
			registry.contracts[contract.ToolName()] = contract
		}
	}
	return registry
}

func (r *ContractRegistry) Get(toolName string) (RoleResultContract, bool) {
	if r == nil {
		return nil, false
	}
	contract, ok := r.contracts[toolName]
	return contract, ok
}

func (r *ContractRegistry) List() []RoleResultContract {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.contracts))
	for name := range r.contracts {
		names = append(names, name)
	}
	sort.Strings(names)
	contracts := make([]RoleResultContract, 0, len(names))
	for _, name := range names {
		contracts = append(contracts, r.contracts[name])
	}
	return contracts
}

func decodeStrict[T any](raw json.RawMessage) (*T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("exactly one JSON object is required")
		}
		return nil, err
	}
	return &value, nil
}

func validateContractContext(expectedRole string, task *model.Task, run *model.Run, input RoleResultInput) error {
	if task == nil {
		return errors.New("task is required")
	}
	if run == nil {
		return errors.New("run is required")
	}
	if input == nil {
		return errors.New("result input is required")
	}
	taskID, runID := input.Identity()
	if strings.TrimSpace(taskID) == "" || taskID != task.TaskID {
		return fmt.Errorf("result task_id %q does not match task %q", taskID, task.TaskID)
	}
	if strings.TrimSpace(runID) == "" || runID != run.RunID {
		return fmt.Errorf("result run_id %q does not match run %q", runID, run.RunID)
	}
	if run.TaskID != task.TaskID {
		return fmt.Errorf("run task_id %q does not match task %q", run.TaskID, task.TaskID)
	}
	if task.CurrentRunID == nil || *task.CurrentRunID != run.RunID {
		return fmt.Errorf("run %q is not current for task %q", run.RunID, task.TaskID)
	}
	if task.Role != expectedRole || run.WorkerRole != expectedRole {
		return fmt.Errorf("tool requires %s, got task role %q and run role %q", expectedRole, task.Role, run.WorkerRole)
	}
	return nil
}

type TestEvidence struct {
	Command string `json:"command"`
	Passed  bool   `json:"passed"`
	Summary string `json:"summary"`
	// Attempts 记录该命令的执行次数(含重跑);0 视同 1。验证任务用机械
	// 重跑消化偶发抖动,逐次留痕让红绿判定可审计。
	Attempts int `json:"attempts,omitempty"`
}

type DeliveryEvidence struct {
	CommitSHA    string   `json:"commit_sha"`
	ChangedFiles []string `json:"changed_files"`
	DiffSummary  string   `json:"diff_summary"`
}

type MergeRequestResult struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	RemoveSourceBranch bool   `json:"remove_source_branch,omitempty"`
}

type DefectResult struct {
	Repository         string `json:"repository"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	DependencyRelation string `json:"dependency_relation,omitempty"`
}

type FailureEvidence struct {
	Files          []string `json:"files,omitempty"`
	FailureExcerpt string   `json:"failure_excerpt,omitempty"`
}

func validateTests(tests []TestEvidence, requirePassed bool) error {
	if len(tests) == 0 {
		return errors.New("tests evidence is required")
	}
	for i, test := range tests {
		if strings.TrimSpace(test.Command) == "" || strings.TrimSpace(test.Summary) == "" {
			return fmt.Errorf("tests[%d] command and summary are required", i)
		}
		if requirePassed && !test.Passed {
			return fmt.Errorf("tests[%d] must pass before change delivery", i)
		}
	}
	return nil
}

func parseTaskContext(task *model.Task) (map[string]any, error) {
	context := map[string]any{}
	if task == nil || strings.TrimSpace(task.ContextJSON) == "" {
		return context, nil
	}
	if err := json.Unmarshal([]byte(task.ContextJSON), &context); err != nil {
		return nil, fmt.Errorf("decode task context: %w", err)
	}
	return context, nil
}

func deliveryTarget(task *model.Task) (sourceBranch string, targetBranch string, err error) {
	context, err := parseTaskContext(task)
	if err != nil {
		return "", "", err
	}
	repos, _ := context["repos"].([]any)
	writableCount := 0
	for _, rawRepo := range repos {
		repo, _ := rawRepo.(map[string]any)
		access, _ := repo["access"].(string)
		if !strings.EqualFold(strings.TrimSpace(access), "writable") {
			continue
		}
		writableCount++
		targetBranch, _ = repo["branch"].(string)
	}
	if writableCount != 1 {
		return "", "", fmt.Errorf("exactly one writable repository is required, got %d", writableCount)
	}
	if strings.TrimSpace(targetBranch) == "" {
		targetBranch = "main"
	}
	return model.DeliveryBranch(task), targetBranch, nil
}

func resultArtifacts(delivery *DeliveryEvidence, tests []TestEvidence, extra map[string]any) map[string]any {
	artifacts := make(map[string]any, len(extra)+4)
	for key, value := range extra {
		artifacts[key] = value
	}
	if delivery != nil {
		artifacts["commit_sha"] = delivery.CommitSHA
		artifacts["changed_files"] = delivery.ChangedFiles
		artifacts["diff"] = delivery.DiffSummary
	}
	if len(tests) > 0 {
		artifacts["test_results"] = tests
	}
	return artifacts
}

func stringField(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return strings.TrimSpace(value)
}

func numberField(object map[string]any, key string) int {
	switch value := object[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func objectField(parent map[string]any, key string) map[string]any {
	value, _ := parent[key].(map[string]any)
	return value
}

func baseInputSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             required,
		"properties":           properties,
		"additionalProperties": false,
	}
}

func strictObjectSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             required,
		"properties":           properties,
		"additionalProperties": false,
	}
}

func testEvidenceArraySchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "只填写作为交付依据的通过验证；已确认与本次改动无关的既有失败（如环境或平台差异导致）写入 summary 说明，不得作为 tests 条目提交",
		"items": strictObjectSchema(
			[]string{"command", "passed", "summary"},
			map[string]any{
				"command":  map[string]any{"type": "string"},
				"passed":   map[string]any{"type": "boolean", "description": "交付验证要求全部通过（passed=true）"},
				"summary":  map[string]any{"type": "string"},
				"attempts": map[string]any{"type": "integer", "description": "该命令的执行次数（含重跑），缺省视为 1"},
			},
		),
	}
}

func deliveryEvidenceSchema() map[string]any {
	return strictObjectSchema(
		[]string{"commit_sha", "changed_files", "diff_summary"},
		map[string]any{
			"commit_sha":    map[string]any{"type": "string"},
			"changed_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"diff_summary":  map[string]any{"type": "string"},
		},
	)
}

func mergeRequestResultSchema() map[string]any {
	return strictObjectSchema(
		[]string{"title", "description"},
		map[string]any{
			"title":                map[string]any{"type": "string"},
			"description":          map[string]any{"type": "string"},
			"remove_source_branch": map[string]any{"type": "boolean"},
		},
	)
}

func defectResultSchema() map[string]any {
	return strictObjectSchema(
		[]string{"repository", "title", "description"},
		map[string]any{
			"repository":          map[string]any{"type": "string"},
			"title":               map[string]any{"type": "string"},
			"description":         map[string]any{"type": "string"},
			"dependency_relation": map[string]any{"type": "string"},
		},
	)
}
