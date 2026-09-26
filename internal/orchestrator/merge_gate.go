package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
)

// 自动合并门禁的裁决取值:merged=平台接受合并;fallback=任一环缺失、证据
// 过期或平台拒绝而回落人工(原因逐项留审计);skipped=查询类瞬时故障,不
// 写审计,等下一轮重评(fail-closed 的同时保留收敛机会)。
const (
	MergeGateDecisionMerged   = "merged"
	MergeGateDecisionFallback = "fallback"
	MergeGateDecisionSkipped  = "skipped"
)

const mergeGateActorKind = "auto_merge_gate"

// mergeGateClientFactory 按仓行构造门禁客户端(读实时头部 + 执行合并);
// merger 返回 nil 表示平台不具备合并能力,门禁回落人工。
type mergeGateClientFactory = func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error)

// RunAutoMergeGate 扫描已完成且从未被评估过的验证任务,逐个评估证据合取,
// 全绿即凭租户仓行 PAT 调平台合并 API(带头部 SHA 断言)。开关关闭时零开销
// 短路;端口未装配(最小装配)同样跳过。
func (o *Orchestrator) RunAutoMergeGate(ctx context.Context, limit int) ([]MergeGateTickResult, error) {
	if limit <= 0 || o.mergeAudits == nil || o.repositoryLookup == nil || o.autoMergeEnabled == nil || o.mergeGateClients == nil {
		return nil, nil
	}
	if !o.autoMergeEnabled(ctx) {
		return nil, nil
	}
	candidates, err := o.tasks.ListSucceededVerificationsWithoutAudit(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list gate candidates: %w", err)
	}
	decisions := make([]MergeGateTickResult, 0, len(candidates))
	for i := range candidates {
		task := &candidates[i]
		decision, reasons, mergedSHA, transientErr := o.evaluateAutoMergeGate(ctx, task)
		if transientErr != nil {
			// 查询类瞬时故障:不写审计(下一轮重评),本轮记 skipped。
			decisions = append(decisions, MergeGateTickResult{
				TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: transientErr.Error(),
			})
			continue
		}
		if writeErr := o.writeMergeGateAudit(ctx, task, decision, reasons, mergedSHA); writeErr != nil {
			decisions = append(decisions, MergeGateTickResult{
				TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: writeErr.Error(),
			})
			continue
		}
		decisions = append(decisions, MergeGateTickResult{
			TaskID: task.TaskID, Decision: decision, Reasons: strings.Join(reasons, "; "),
		})
	}
	return decisions, nil
}

// DefaultMergeGateClientFactory 按仓行平台身份构造平台客户端,拆出读/合
// 两个能力面;平台未实现合并不报错(merger=nil),由评估器回落人工。
func DefaultMergeGateClientFactory(repo *registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error) {
	client, err := registeredrepo.CodeHostClient(repo.Provider, repo.BaseURL, repo.Token, nil)
	if err != nil {
		return nil, nil, err
	}
	reader, _ := client.(codehost.MergeRequestReader)
	if reader == nil {
		return nil, nil, fmt.Errorf("platform client %s does not expose merge request reads", repo.Provider)
	}
	merger, _ := client.(codehost.MergeRequestMerger)
	return reader, merger, nil
}

// evaluateAutoMergeGate 评估单条验证任务的证据合取。返回值:决策、逐项
// 理由、合并产出的平台提交号、瞬时错误。瞬时错误(平台读失败、审计写失败、
// 网络歧义)不产生决策,由调用方留待下一轮——fail-closed 且保留收敛机会。
func (o *Orchestrator) evaluateAutoMergeGate(ctx context.Context, task *model.Task) (string, []string, string, error) {
	var taskContext struct {
		ParentTaskID string `json:"parent_task_id"`
		MergeRequest struct {
			IID     int    `json:"iid"`
			HeadSHA string `json:"head_sha"`
		} `json:"merge_request"`
		Project struct {
			Path string `json:"path"`
		} `json:"project"`
	}
	if err := json.Unmarshal([]byte(task.ContextJSON), &taskContext); err != nil {
		return "", nil, "", fmt.Errorf("decode verification context: %w", err)
	}
	headSHA := strings.TrimSpace(taskContext.MergeRequest.HeadSHA)
	projectPath := strings.TrimSpace(taskContext.Project.Path)
	if projectPath == "" {
		projectPath = task.TargetRepo
	}
	reasons := make([]string, 0, 4)

	// 防线二的前半:tester 证据本体必须绿且绑定当前头部(系统侧绑定,
	// 不信任 agent 回显)。
	verifierVerdict, headBinding, err := o.gateArtifactValues(ctx, task.TaskID, "verification_verdict", "verification_head_sha")
	if err != nil {
		return "", nil, "", err
	}
	if verifierVerdict != "verification_passed" {
		reasons = append(reasons, fmt.Sprintf("tester verdict is %q, not passed", verifierVerdict))
		return MergeGateDecisionFallback, reasons, "", nil
	}
	if headSHA == "" || headBinding == "" || headBinding != headSHA {
		reasons = append(reasons, fmt.Sprintf("tester head binding %q does not match task head %q", headBinding, headSHA))
		return MergeGateDecisionFallback, reasons, "", nil
	}

	// reviewer 放行且审查的正是同一头部:SHA 不一致即审查证据过期。
	if strings.TrimSpace(taskContext.ParentTaskID) == "" {
		reasons = append(reasons, "verification context has no reviewer lineage")
		return MergeGateDecisionFallback, reasons, "", nil
	}
	parent, err := o.tasks.GetByTaskID(ctx, taskContext.ParentTaskID)
	if err != nil {
		return "", nil, "", fmt.Errorf("load reviewer task %s: %w", taskContext.ParentTaskID, err)
	}
	var parentContext struct {
		MergeRequest struct {
			HeadSHA string `json:"head_sha"`
		} `json:"merge_request"`
	}
	if err := json.Unmarshal([]byte(parent.ContextJSON), &parentContext); err != nil {
		return "", nil, "", fmt.Errorf("decode reviewer context: %w", err)
	}
	reviewerVerdict, _, err := o.gateArtifactValues(ctx, parent.TaskID, "review_verdict", "")
	if err != nil {
		return "", nil, "", err
	}
	if reviewerVerdict != "approved" && reviewerVerdict != "no_new_changes" {
		reasons = append(reasons, fmt.Sprintf("reviewer verdict is %q, not approving", reviewerVerdict))
		return MergeGateDecisionFallback, reasons, "", nil
	}
	if strings.TrimSpace(parentContext.MergeRequest.HeadSHA) != headSHA {
		reasons = append(reasons, fmt.Sprintf("reviewer head %q differs from verification head %q", parentContext.MergeRequest.HeadSHA, headSHA))
		return MergeGateDecisionFallback, reasons, "", nil
	}

	if task.RepositoryID == nil {
		reasons = append(reasons, "verification task is not bound to a repository")
		return MergeGateDecisionFallback, reasons, "", nil
	}
	repoRow, err := o.repositoryLookup.Get(ctx, *task.RepositoryID)
	if err != nil {
		return "", nil, "", fmt.Errorf("lookup repository %s: %w", *task.RepositoryID, err)
	}
	reader, merger, err := o.mergeGateClients(repoRow)
	if err != nil {
		return "", nil, "", fmt.Errorf("build gate client: %w", err)
	}
	if merger == nil {
		reasons = append(reasons, fmt.Sprintf("platform %s does not expose a merge capability", repoRow.Provider))
		return MergeGateDecisionFallback, reasons, "", nil
	}

	// 防线二的后半:平台实时头部必须仍等于证据头部(消费校验)。
	mergeRequest, err := reader.GetMergeRequest(ctx, projectPath, taskContext.MergeRequest.IID)
	if err != nil {
		return "", nil, "", fmt.Errorf("read live merge request head: %w", err)
	}
	if strings.TrimSpace(mergeRequest.HeadSHA) != headSHA {
		reasons = append(reasons, fmt.Sprintf("live head %q moved past evidence head %q", mergeRequest.HeadSHA, headSHA))
		return MergeGateDecisionFallback, reasons, "", nil
	}

	// 防线三:合并调用再次断言头部,读合之间头部漂移由平台拒合。
	result, err := merger.MergePullRequest(ctx, projectPath, taskContext.MergeRequest.IID, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: headSHA,
	})
	if err != nil {
		if errors.Is(err, codehost.ErrMergeHeadMismatch) || errors.Is(err, codehost.ErrMergeRejected) {
			reasons = append(reasons, fmt.Sprintf("platform rejected the merge: %v", err))
			return MergeGateDecisionFallback, reasons, "", nil
		}
		return "", nil, "", fmt.Errorf("merge pull request: %w", err)
	}
	if !result.Merged {
		reasons = append(reasons, "platform returned an unmerged result")
		return MergeGateDecisionFallback, reasons, "", nil
	}
	reasons = append(reasons,
		fmt.Sprintf("tester verdict passed at head %s", headSHA),
		fmt.Sprintf("reviewer verdict %q at the same head", reviewerVerdict),
		fmt.Sprintf("platform merged at commit %s", result.MergedCommitSHA),
	)
	return MergeGateDecisionMerged, reasons, result.MergedCommitSHA, nil
}

// gateArtifactValues 按名字读取至多两个 verdict/绑定 artifact 并解码
// value 字段;缺失条目返回空串(调用方按缺失裁决)。第二名可传空跳过。
func (o *Orchestrator) gateArtifactValues(ctx context.Context, taskID, firstName, secondName string) (string, string, error) {
	if o.verificationArtifacts == nil {
		return "", "", fmt.Errorf("artifact reader is required for gate evidence")
	}
	read := func(name string) (string, error) {
		artifacts, err := o.verificationArtifacts.ListByTaskAndName(ctx, taskID, "generic", name)
		if err != nil {
			return "", fmt.Errorf("read artifact %s: %w", name, err)
		}
		if len(artifacts) == 0 {
			return "", nil
		}
		var metadata struct {
			Value any `json:"value"`
		}
		if err := json.Unmarshal([]byte(artifacts[len(artifacts)-1].MetadataJSON), &metadata); err != nil {
			return "", nil
		}
		text, _ := metadata.Value.(string)
		return strings.TrimSpace(text), nil
	}
	first, err := read(firstName)
	if err != nil {
		return "", "", err
	}
	if secondName == "" {
		return first, "", nil
	}
	second, err := read(secondName)
	if err != nil {
		return "", "", err
	}
	return first, second, nil
}

func (o *Orchestrator) writeMergeGateAudit(ctx context.Context, task *model.Task, decision string, reasons []string, mergedCommitSHA string) error {
	var taskContext struct {
		MergeRequest struct {
			HeadSHA string `json:"head_sha"`
		} `json:"merge_request"`
	}
	_ = json.Unmarshal([]byte(task.ContextJSON), &taskContext)
	repositoryID := ""
	if task.RepositoryID != nil {
		repositoryID = *task.RepositoryID
	}
	runID := ""
	if task.CurrentRunID != nil {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	reasonsJSON, err := json.Marshal(reasons)
	if err != nil {
		return fmt.Errorf("marshal gate reasons: %w", err)
	}
	if err := o.mergeAudits.Create(ctx, &model.MergeGateAudit{
		AuditID:         model.NewID("mga"),
		TaskID:          task.TaskID,
		RunID:           runID,
		RepositoryID:    repositoryID,
		HeadSHA:         strings.TrimSpace(taskContext.MergeRequest.HeadSHA),
		Decision:        decision,
		ReasonsJSON:     string(reasonsJSON),
		MergedCommitSHA: mergedCommitSHA,
		ActorKind:       mergeGateActorKind,
	}); err != nil {
		return fmt.Errorf("write merge gate audit: %w", err)
	}
	return nil
}
