package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	storerepo "sdlc-coordinator/internal/store/repository"
)

// 自动合并门禁的裁决取值:merged=平台接受合并(含回读证实已合并);fallback=
// 任一环缺失、证据过期或平台确定性拒绝而回落人工(原因逐项留审计+IM 触点);
// skipped=查询类瞬时故障,不写审计,等下一轮重评(fail-closed 的同时保留收敛
// 机会)。确定性故障必须归 fallback:留在 skipped 会形成每轮重扫的静默死信
// 队列,还会占满取数窗口饿死后续任务。
const (
	MergeGateDecisionMerged   = "merged"
	MergeGateDecisionFallback = "fallback"
	MergeGateDecisionSkipped  = "skipped"
)

const (
	mergeGateActorKind        = "auto_merge_gate"
	mergeGateFallbackIMPrefix = "merge-gate-fallback-im:"
	// mergeGateLeaseScope/TTL 是门禁评估+合并的跨实例执行权范围:证据合取
	// 评估无副作用可重做,但合并调用不可双发,故租约覆盖整个评估+合并。
	mergeGateLeaseScope = "merge-gate:"
	mergeGateLeaseTTL   = 5 * time.Minute
)

const defaultMergeGateLimit = 5

// mergeGateClientFactory 按仓行构造门禁客户端(读实时头部 + 执行合并);
// merger 返回 nil 表示平台不具备合并能力,门禁回落人工。
type mergeGateClientFactory = func(*registeredrepo.RegisteredRepository) (codehost.MergeRequestReader, codehost.MergeRequestMerger, error)

// gateEvaluation 是单条验证任务的评估产物:确定性结果(merged/fallback)
// 必写审计;Transient 非空表示查询/网络类故障,本轮跳过、不写审计。
type gateEvaluation struct {
	Decision        string
	Reasons         []string
	HeadSHA         string
	ProjectPath     string
	MergeRequestIID int
	MergedCommitSHA string
	// NotifyHuman 只在证据链全绿、卡点发生在平台层(实时头部漂移/平台拒绝/
	// 能力缺失)时为真:红验证已有 MR 报告覆盖、血缘过期自有新链在跑,这类
	// fallback 发 IM 是重复触达甚至错误建议,不通知。
	NotifyHuman bool
	Transient   error
}

// RunAutoMergeGate 扫描已完成且从未被评估过的验证任务,逐个评估证据合取,
// 全绿即凭租户仓行 PAT 调平台合并 API(带头部 SHA 断言)。开关关闭时零开销
// 短路;端口未装配(最小装配)同样跳过;limit 未传按默认批量。
func (o *Orchestrator) RunAutoMergeGate(ctx context.Context, limit int) ([]MergeGateTickResult, error) {
	if o.mergeAudits == nil || o.repositoryLookup == nil || o.autoMergeEnabled == nil || o.mergeGateClients == nil {
		return nil, nil
	}
	if !o.autoMergeEnabled(ctx) {
		return nil, nil
	}
	o.mergeGateMu.Lock()
	defer o.mergeGateMu.Unlock()
	if limit <= 0 {
		limit = defaultMergeGateLimit
	}
	candidates, err := o.tasks.ListSucceededVerificationsWithoutAuditAfter(ctx, o.mergeGateCursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list gate candidates: %w", err)
	}
	decisions := make([]MergeGateTickResult, 0, len(candidates))
	for i := range candidates {
		task := &candidates[i]
		o.mergeGateCursor = task.ID
		// 跨实例执行权:双实例同评同任务时只有赢家走到合并调用;败者
		// skipped 退出,审计唯一约束仍是最终防重防线。
		lease, won, leaseErr := o.acquireWorkItemLease(ctx, mergeGateLeaseScope+task.TaskID, "merge-gate", mergeGateLeaseTTL)
		if leaseErr != nil {
			decisions = append(decisions, MergeGateTickResult{
				TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: leaseErr.Error(),
			})
			continue
		}
		if !won {
			decisions = append(decisions, MergeGateTickResult{
				TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: "merge gate evaluation is held by another instance",
			})
			continue
		}
		// 租约获胜后再查一次审计:上一赢家可能在本次 List 与抢租约之间完成
		// 全链(评估+合并+审计+释放),审计唯一约束是最终防重,这里补一个
		// 廉价读避免对其重复执行合并调用。
		if _, auditErr := o.mergeAudits.GetByTaskID(ctx, task.TaskID); auditErr == nil {
			o.releaseWorkItemLease(lease)
			continue
		}
		result := o.evaluateAutoMergeGate(ctx, task, lease)
		o.releaseWorkItemLease(lease)
		if result.Transient != nil {
			// 查询类瞬时故障:不写审计(下一轮重评),本轮记 skipped 并留
			// 日志——持续瞬态失败时每轮可见,不静默。
			o.logger.Warn("auto merge gate deferred task to next tick",
				zap.String("task_id", task.TaskID),
				zap.NamedError("transient", result.Transient),
			)
			decisions = append(decisions, MergeGateTickResult{
				TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: result.Transient.Error(),
			})
			continue
		}
		if result.Decision == MergeGateDecisionFallback && result.NotifyHuman {
			// 先持久化通知再写终态审计。若通知落库失败,保留无审计
			// 状态让下一 tick 重试,避免终态防重先截断人工触点。
			if notifyErr := o.materializeGateFallbackIM(ctx, task, result); notifyErr != nil {
				decisions = append(decisions, MergeGateTickResult{
					TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: notifyErr.Error(),
				})
				continue
			}
		}
		if writeErr := o.writeMergeGateAudit(ctx, task, result); writeErr != nil {
			decisions = append(decisions, MergeGateTickResult{
				TaskID: task.TaskID, Decision: MergeGateDecisionSkipped, Error: writeErr.Error(),
			})
			continue
		}
		decisions = append(decisions, MergeGateTickResult{
			TaskID: task.TaskID, HeadSHA: result.HeadSHA, Decision: result.Decision, Reasons: strings.Join(result.Reasons, "; "),
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

// evaluateAutoMergeGate 评估单条验证任务的证据合取。确定性失败(上下文不可
// 读、血缘/仓行缺失、平台 4xx 拒绝、头部漂移)一律 fallback+审计,只有查询
// 类瞬时故障才返回 Transient 留待下轮。
func (o *Orchestrator) evaluateAutoMergeGate(ctx context.Context, task *model.Task, lease model.WorkItemLease) gateEvaluation {
	result := gateEvaluation{}
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
		result.Decision = MergeGateDecisionFallback
		result.Reasons = []string{fmt.Sprintf("verification context is unreadable: %v", err)}
		return result
	}
	result.HeadSHA = strings.TrimSpace(taskContext.MergeRequest.HeadSHA)
	result.MergeRequestIID = taskContext.MergeRequest.IID
	result.ProjectPath = strings.TrimSpace(taskContext.Project.Path)
	if result.ProjectPath == "" {
		result.ProjectPath = task.TargetRepo
	}
	reasons := make([]string, 0, 4)

	// 防线二的前半:tester 证据本体必须绿且绑定当前头部(系统侧绑定,
	// 不信任 agent 回显)。
	verifierVerdict, headBinding, err := o.gateArtifactValues(ctx, task.TaskID, "verification_verdict", "verification_head_sha")
	if err != nil {
		result.Transient = err
		return result
	}
	fallback := func(reason string) gateEvaluation {
		result.Decision = MergeGateDecisionFallback
		result.Reasons = append(reasons, reason)
		return result
	}
	if verifierVerdict != "verification_passed" {
		return fallback(fmt.Sprintf("tester verdict is %q, not passed", verifierVerdict))
	}
	if result.HeadSHA == "" || headBinding == "" || headBinding != result.HeadSHA {
		return fallback(fmt.Sprintf("tester head binding %q does not match task head %q", headBinding, result.HeadSHA))
	}

	// reviewer 放行且审查的正是同一头部:此比对是血缘簿记一致性,真正拦
	// 头部漂移的是后面的平台实时头部校验与合并 SHA 断言。
	if strings.TrimSpace(taskContext.ParentTaskID) == "" {
		return fallback("verification context has no reviewer lineage")
	}
	parent, err := o.tasks.GetByTaskID(ctx, taskContext.ParentTaskID)
	if err != nil {
		if isLookupNotFound(err) {
			return fallback(fmt.Sprintf("reviewer task %s no longer exists", taskContext.ParentTaskID))
		}
		result.Transient = fmt.Errorf("load reviewer task %s: %w", taskContext.ParentTaskID, err)
		return result
	}
	var parentContext struct {
		MergeRequest struct {
			HeadSHA string `json:"head_sha"`
		} `json:"merge_request"`
	}
	if err := json.Unmarshal([]byte(parent.ContextJSON), &parentContext); err != nil {
		return fallback(fmt.Sprintf("reviewer context is unreadable: %v", err))
	}
	reviewerVerdict, _, err := o.gateArtifactValues(ctx, parent.TaskID, "review_verdict", "")
	if err != nil {
		result.Transient = err
		return result
	}
	if reviewerVerdict != "approved" {
		return fallback(fmt.Sprintf("reviewer verdict is %q, not approving", reviewerVerdict))
	}
	if strings.TrimSpace(parentContext.MergeRequest.HeadSHA) != result.HeadSHA {
		return fallback(fmt.Sprintf("reviewer head %q differs from verification head %q", parentContext.MergeRequest.HeadSHA, result.HeadSHA))
	}
	// 证据链到此全绿:之后任何卡点都发生在平台层,值得人工触达。
	result.NotifyHuman = true

	if task.RepositoryID == nil {
		return fallback("verification task is not bound to a repository")
	}
	repoRow, err := o.repositoryLookup.Get(ctx, *task.RepositoryID)
	if err != nil {
		if isLookupNotFound(err) {
			return fallback(fmt.Sprintf("repository row %s no longer exists", *task.RepositoryID))
		}
		result.Transient = fmt.Errorf("lookup repository %s: %w", *task.RepositoryID, err)
		return result
	}
	if repoRow.Status != registeredrepo.StatusEnabled {
		return fallback(fmt.Sprintf("repository status is %q, not enabled", repoRow.Status))
	}
	reader, merger, err := o.mergeGateClients(repoRow)
	if err != nil {
		return fallback(fmt.Sprintf("gate client unavailable: %v", err))
	}
	if merger == nil {
		return fallback(fmt.Sprintf("platform %s does not expose a merge capability", repoRow.Provider))
	}

	// 防线二的后半:平台实时头部必须仍等于证据头部(消费校验)。平台已
	// 显示合并时按 merged 补审计——这是合并成功但审计写失败轮次的
	// 真相恢复路径,防止重复合并调用把审计误记成 fallback。
	mergeRequest, err := reader.GetMergeRequest(ctx, result.ProjectPath, result.MergeRequestIID)
	if err != nil {
		if isDeterministicRejection(err) {
			return fallback(fmt.Sprintf("platform rejected the read: %v", err))
		}
		result.Transient = fmt.Errorf("read live merge request head: %w", err)
		return result
	}
	if mergeRequest.Merged {
		result.Decision = MergeGateDecisionMerged
		result.MergedCommitSHA = strings.TrimSpace(mergeRequest.MergeCommitSHA)
		result.Reasons = append(reasons, fmt.Sprintf("platform already reports the merge request as merged at %s", result.MergedCommitSHA))
		return result
	}
	if strings.TrimSpace(mergeRequest.HeadSHA) != result.HeadSHA {
		return fallback(fmt.Sprintf("live head %q moved past evidence head %q", mergeRequest.HeadSHA, result.HeadSHA))
	}

	// 管理员可能在实时头部读取期间禁用仓库。合并前重新读取组织开关，
	// 避免继续使用此前取出的仓库凭据执行写操作。
	latestRepo, err := o.repositoryLookup.Get(ctx, *task.RepositoryID)
	if err != nil {
		if isLookupNotFound(err) {
			return fallback(fmt.Sprintf("repository row %s no longer exists before merge", *task.RepositoryID))
		}
		result.Transient = fmt.Errorf("recheck repository %s before merge: %w", *task.RepositoryID, err)
		return result
	}
	if latestRepo.Status != registeredrepo.StatusEnabled {
		return fallback(fmt.Sprintf("repository status is %q before merge, not enabled", latestRepo.Status))
	}

	// 防线三:合并调用再次断言头部,读合之间头部漂移由平台拒合。
	runID := ""
	if task.CurrentRunID != nil {
		runID = *task.CurrentRunID
	}
	admission, err := o.admitExternalAction(ctx, ActionAdmissionRequest{TaskID: task.TaskID, RunID: runID, Kind: model.ActionAutoMerge, ActionKey: task.TaskID, Attempt: 1}, lease)
	if err != nil {
		result.NotifyHuman = false
		result.Transient = err
		return result
	}
	mergeResult, err := merger.MergePullRequest(ctx, result.ProjectPath, result.MergeRequestIID, codehost.MergePullRequestRequest{
		ExpectedHeadSHA: result.HeadSHA,
	})
	if err != nil {
		if isDeterministicRejection(err) {
			// 拒绝先回读真相:合并可能已在更早的歧义轮次生效。
			recheck, readErr := reader.GetMergeRequest(ctx, result.ProjectPath, result.MergeRequestIID)
			if readErr == nil && recheck.Merged {
				result.Decision = MergeGateDecisionMerged
				result.MergedCommitSHA = strings.TrimSpace(recheck.MergeCommitSHA)
				result.Reasons = append(reasons, fmt.Sprintf("merge call was rejected but the platform reports the merge request merged at %s", result.MergedCommitSHA))
				return result
			}
			if proofErr := o.confirmActionNotSent(ctx, admission); proofErr != nil {
				result.Transient = proofErr
				return result
			}
			return fallback(fmt.Sprintf("platform rejected the merge: %v", err))
		}
		result.Transient = fmt.Errorf("merge pull request: %w", err)
		return result
	}
	if !mergeResult.Merged {
		if proofErr := o.confirmActionNotSent(ctx, admission); proofErr != nil {
			result.Transient = proofErr
			return result
		}
		return fallback("platform returned an unmerged result")
	}
	result.Decision = MergeGateDecisionMerged
	result.MergedCommitSHA = strings.TrimSpace(mergeResult.MergedCommitSHA)
	result.Reasons = append(reasons,
		fmt.Sprintf("tester verdict passed at head %s", result.HeadSHA),
		fmt.Sprintf("reviewer verdict %q at the same head", reviewerVerdict),
		fmt.Sprintf("platform merged at commit %s", result.MergedCommitSHA),
	)
	return result
}

// isDeterministicRejection 判定平台侧确定性拒绝:门禁两个哨兵覆盖 409 头部
// 断言失配与 4xx 家族(404 删除/401 凭据/403 保护规则等);5xx、429/408 与
// 网络类走 Ambiguous/TransientRejection,不属于此类。
func isDeterministicRejection(err error) bool {
	return errors.Is(err, codehost.ErrMergeHeadMismatch) || errors.Is(err, codehost.ErrMergeRejected)
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

func (o *Orchestrator) writeMergeGateAudit(ctx context.Context, task *model.Task, result gateEvaluation) error {
	repositoryID := ""
	if task.RepositoryID != nil {
		repositoryID = *task.RepositoryID
	}
	runID := ""
	if task.CurrentRunID != nil {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	reasonsJSON, err := json.Marshal(result.Reasons)
	if err != nil {
		return fmt.Errorf("marshal gate reasons: %w", err)
	}
	if err := o.mergeAudits.Create(ctx, &model.MergeGateAudit{
		AuditID:         model.NewID("mga"),
		TaskID:          task.TaskID,
		RunID:           runID,
		RepositoryID:    repositoryID,
		ProjectPath:     result.ProjectPath,
		MergeRequestIID: result.MergeRequestIID,
		HeadSHA:         result.HeadSHA,
		Decision:        result.Decision,
		ReasonsJSON:     string(reasonsJSON),
		MergedCommitSHA: result.MergedCommitSHA,
		ActorKind:       mergeGateActorKind,
	}); err != nil {
		return fmt.Errorf("write merge gate audit: %w", err)
	}
	return nil
}

// materializeGateFallbackIM 把门禁回落物化为 IM 通知。去重按验证任务,
// 已有即成功;持久化失败向上返回,让门禁保留可重试状态。
func (o *Orchestrator) materializeGateFallbackIM(ctx context.Context, task *model.Task, result gateEvaluation) error {
	if o.effects == nil || task == nil {
		return nil
	}
	dedupeKey := mergeGateFallbackIMPrefix + task.TaskID
	if _, err := o.effects.GetByDedupeKey(ctx, dedupeKey); err == nil {
		return nil
	} else if !isLookupNotFound(err) {
		return fmt.Errorf("read merge gate fallback IM effect: %w", err)
	}
	body := fmt.Sprintf(
		"自动合并门禁已回落人工处理。\n\n- Task: `%s`\n- MR: `%s` iid %d\n- Head: `%s`\n- 原因:\n%s\n\n证据链与验证任务见执行观测页;人工核验后可直接合并,或按原因处理后重推。",
		task.TaskID,
		result.ProjectPath,
		result.MergeRequestIID,
		result.HeadSHA,
		joinReasonsAsQuotes(result.Reasons),
	)
	requestJSON, err := json.Marshal(map[string]string{"body": body, "format": "markdown"})
	if err != nil {
		return fmt.Errorf("encode merge gate fallback IM effect: %w", err)
	}
	runID := ""
	if task.CurrentRunID != nil {
		runID = strings.TrimSpace(*task.CurrentRunID)
	}
	if err := o.effects.UpsertByDedupeKey(ctx, &model.Effect{
		TaskID:      task.TaskID,
		RunID:       runID,
		DedupeKey:   dedupeKey,
		Kind:        leaseRecoveryIMEffectKind,
		Target:      leaseRecoveryIMEffectTarget,
		Critical:    false,
		Status:      model.EffectStatusPending,
		RequestJSON: string(requestJSON),
	}); err != nil {
		return fmt.Errorf("persist merge gate fallback IM effect: %w", err)
	}
	return nil
}

func joinReasonsAsQuotes(reasons []string) string {
	lines := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		lines = append(lines, "- "+reason)
	}
	return strings.Join(lines, "\n")
}

// isLookupNotFound 统一识别仓储层的"查无"哨兵(具体仓统一映射为
// ErrNotFound;兼容直接透出 gorm 裸错误的实现)。
func isLookupNotFound(err error) bool {
	return errors.Is(err, storerepo.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound)
}

// logGateScanFailure 把门禁扫描失败降级为日志:门禁是插在必经主链路之前的
// 可选阶段,查询失败不得拖住同轮的恢复/派发/效果执行(下一轮自然重试)。
func (o *Orchestrator) logGateScanFailure(err error) {
	o.logger.Warn("auto merge gate scan deferred to next tick", zap.Error(err))
}
