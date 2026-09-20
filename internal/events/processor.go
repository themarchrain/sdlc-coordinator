package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

type InboundEventStore interface {
	Create(context.Context, *model.InboundEvent) error
	Update(context.Context, *model.InboundEvent) error
	GetByDedupeKey(context.Context, string) (*model.InboundEvent, error)
}

type TaskCreator interface {
	CreateTask(context.Context, *model.Task) (*model.Task, error)
}

type DeveloperTaskResumer interface {
	ResumeDeveloperTask(context.Context, model.DeveloperResumeRequest) (*model.Task, error)
}

// Normalizer 把某一平台的 webhook 载荷规范化为内部事件;每种代码托管平台一个实现。
type Normalizer interface {
	Source() string
	Receipt(raw []byte, metadata WebhookMetadata) WebhookReceipt
	Normalize(raw []byte) (*NormalizationResult, error)
}

// WebhookMetadata 承载平台无关的 webhook 头信息(EventType 对应 X-Gitlab-Event /
// X-Gitea-Event,DeliveryID 对应 X-Gitlab-Event-UUID / X-Gitea-Delivery)。
type WebhookMetadata struct {
	EventType   string
	DeliveryID  string
	InstanceURL string
}

// WebhookReceipt 是入站事件的幂等回执标识。
type WebhookReceipt struct {
	DedupeKey   string
	EventType   string
	EventAction string
	ExternalID  string
}

type MergeRequestReworkHandler interface {
	StartOrResumeMergeRequestRework(context.Context, model.MergeRequestReworkRequest) (*model.Task, error)
	ApplyMergeRequestLifecycle(context.Context, model.MergeRequestLifecycleRequest) (*model.Task, error)
}

type Processor struct {
	events                  InboundEventStore
	taskCreator             TaskCreator
	gitlabNormalizerOptions []GitLabNormalizerOption
	// gitlabReworkLookup 在每次 GitLab webhook 投递时以请求 ctx 现取接续
	// 开发白名单(组织段配置保存后的下一个事件即用新值,热生效),物化为
	// 本次投递的 normalizer 选项。
	gitlabReworkLookup func(context.Context) []int
	taskFactory        TaskFactory
	repos              RepositoryLookup
}

type RepositoryLookup interface {
	// GetByProjectID 按 provider + project id 定位仓:数字 project id 跨平台
	// 实例可能撞号,provider 作用域防跨平台串仓。provider 取事件 Source
	// (Normalizer.Source() 三值恰为 gitlab/gitea/github,与仓行 provider 静态一致)。
	GetByProjectID(ctx context.Context, provider, projectID string) (*registeredrepo.RegisteredRepository, error)
}

type ProcessResult struct {
	Event        *model.InboundEvent `json:"event"`
	Task         *model.Task         `json:"task,omitempty"`
	Ignored      bool                `json:"ignored"`
	IgnoreReason string              `json:"ignore_reason,omitempty"`
	Duplicate    bool                `json:"duplicate"`
}

type GitLabWebhookMetadata struct {
	EventName   string
	EventUUID   string
	WebhookUUID string
	InstanceURL string
}

type GitLabWebhookOption func(*gitLabWebhookOptions)

type gitLabWebhookOptions struct {
	metadata GitLabWebhookMetadata
}

func WithGitLabWebhookMetadata(metadata GitLabWebhookMetadata) GitLabWebhookOption {
	return func(options *gitLabWebhookOptions) {
		options.metadata = metadata
	}
}

// NewProcessor 的 normalizer 选项在每次 GitLab webhook 处理时重新物化:
// 选项里挂接续开发白名单查找函数时,组织段配置保存后的下一个事件即用新值。
func NewProcessor(events InboundEventStore, taskCreator TaskCreator, normalizerOptions ...GitLabNormalizerOption) *Processor {
	return &Processor{
		events:                  events,
		taskCreator:             taskCreator,
		gitlabNormalizerOptions: normalizerOptions,
		taskFactory:             NewTaskFactory(),
	}
}

// SetGitLabReworkLookup 注册接续开发白名单的动态来源:每次 GitLab webhook
// 投递时以请求 ctx 调用一次,结果物化进本次投递的 normalizer。读取跟随
// 请求取消链,不脱离 webhook 的超时控制。
func (p *Processor) SetGitLabReworkLookup(lookup func(context.Context) []int) {
	if p == nil {
		return
	}
	p.gitlabReworkLookup = lookup
}

func (p *Processor) SetRepositoryLookup(lookup RepositoryLookup) {
	if p == nil {
		return
	}
	p.repos = lookup
}

func (p *Processor) ProcessGitLabWebhook(ctx context.Context, raw []byte, opts ...GitLabWebhookOption) (*ProcessResult, error) {
	options := gitLabWebhookOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	normalizerOptions := p.gitlabNormalizerOptions
	if p.gitlabReworkLookup != nil {
		normalizerOptions = append(append([]GitLabNormalizerOption{}, normalizerOptions...),
			WithGitLabReworkAllowedUserIDs(p.gitlabReworkLookup(ctx)))
	}
	return p.ProcessWebhook(ctx, NewGitLabNormalizer(normalizerOptions...), raw, WebhookMetadata{
		EventType:   options.metadata.EventName,
		DeliveryID:  options.metadata.EventUUID,
		InstanceURL: options.metadata.InstanceURL,
	})
}

func (p *Processor) ProcessWebhook(ctx context.Context, normalizer Normalizer, raw []byte, metadata WebhookMetadata) (*ProcessResult, error) {
	if p == nil || p.events == nil {
		return nil, errors.New("inbound event store is required")
	}
	if p.taskCreator == nil {
		return nil, errors.New("task creator is required")
	}
	if normalizer == nil {
		return nil, errors.New("normalizer is required")
	}
	if len(raw) == 0 {
		return nil, errors.New("webhook payload is required")
	}

	receipt := normalizer.Receipt(raw, metadata)
	duplicate := false
	existing, err := p.events.GetByDedupeKey(ctx, receipt.DedupeKey)
	if err == nil {
		duplicate = true
		switch existing.Status {
		case model.InboundEventStatusReceived, model.InboundEventStatusNormalized, model.InboundEventStatusDeferred:
			// Resume the persisted receipt below. The stored payload is the source of
			// truth because a replay with the same delivery UUID may carry a different body.
			raw = []byte(existing.PayloadJSON)
		case model.InboundEventStatusIgnored, model.InboundEventStatusTaskCreated, model.InboundEventStatusFailed:
			return &ProcessResult{
				Event:        existing,
				Ignored:      existing.Status == model.InboundEventStatusIgnored,
				IgnoreReason: existing.ErrorMessage,
				Duplicate:    true,
			}, nil
		default:
			return nil, fmt.Errorf("unsupported inbound event status %q", existing.Status)
		}
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	inbound := existing
	if inbound == nil {
		inbound = &model.InboundEvent{
			DedupeKey:   receipt.DedupeKey,
			Source:      normalizer.Source(),
			EventType:   receipt.EventType,
			EventAction: receipt.EventAction,
			ExternalID:  receipt.ExternalID,
			Status:      model.InboundEventStatusReceived,
			PayloadJSON: string(raw),
		}
		if createErr := p.events.Create(ctx, inbound); createErr != nil {
			// A concurrent delivery with the same UUID can win the unique receipt
			// insert after our initial lookup. Re-read and resume that receipt.
			concurrent, lookupErr := p.events.GetByDedupeKey(ctx, receipt.DedupeKey)
			if lookupErr != nil {
				return nil, createErr
			}
			duplicate = true
			inbound = concurrent
			switch inbound.Status {
			case model.InboundEventStatusReceived, model.InboundEventStatusNormalized, model.InboundEventStatusDeferred:
				raw = []byte(inbound.PayloadJSON)
			case model.InboundEventStatusIgnored, model.InboundEventStatusTaskCreated, model.InboundEventStatusFailed:
				return &ProcessResult{
					Event:        inbound,
					Ignored:      inbound.Status == model.InboundEventStatusIgnored,
					IgnoreReason: inbound.ErrorMessage,
					Duplicate:    true,
				}, nil
			default:
				return nil, fmt.Errorf("unsupported inbound event status %q", inbound.Status)
			}
		}
	}

	var normalizedEvent *NormalizedEvent
	if inbound.Status == model.InboundEventStatusReceived {
		normalization, err := normalizer.Normalize(raw)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
		if normalization.Ignored {
			inbound.Status = model.InboundEventStatusIgnored
			inbound.ErrorMessage = normalization.IgnoreReason
			if err := p.events.Update(ctx, inbound); err != nil {
				return nil, err
			}
			return &ProcessResult{
				Event:        inbound,
				Ignored:      true,
				IgnoreReason: normalization.IgnoreReason,
				Duplicate:    duplicate,
			}, nil
		}

		normalizedJSON, err := json.Marshal(normalization.Event)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, fmt.Errorf("marshal normalized event: %w", err))
		}
		inbound.NormalizedJSON = string(normalizedJSON)
		inbound.Status = model.InboundEventStatusNormalized
		if err := p.events.Update(ctx, inbound); err != nil {
			return nil, err
		}
		normalizedEvent = normalization.Event
	} else {
		normalizedEvent = &NormalizedEvent{}
		if err := json.Unmarshal([]byte(inbound.NormalizedJSON), normalizedEvent); err != nil {
			return nil, p.failInboundEvent(ctx, inbound, fmt.Errorf("unmarshal normalized event: %w", err))
		}
	}

	if denied, err := p.denyUnregisteredRepository(ctx, inbound, normalizedEvent, duplicate); err != nil {
		return nil, err
	} else if denied != nil {
		return denied, nil
	}

	var createdTask *model.Task
	if normalizedEvent.Kind == EventKindDeveloperResumeRequested {
		resumer, ok := p.taskCreator.(DeveloperTaskResumer)
		if !ok {
			return nil, p.failInboundEvent(ctx, inbound, errors.New("developer task resumer is required"))
		}
		request, err := developerResumeRequestFromEvent(*normalizedEvent)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
		createdTask, err = resumer.ResumeDeveloperTask(ctx, request)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrTaskNotClaimable) {
				return p.ignoreInboundEvent(ctx, inbound, err.Error(), duplicate)
			}
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
	} else if normalizedEvent.Kind == EventKindMergeRequestReworkRequested {
		handler, ok := p.taskCreator.(MergeRequestReworkHandler)
		if !ok {
			return nil, p.failInboundEvent(ctx, inbound, errors.New("merge request rework handler is required"))
		}
		request, err := mergeRequestReworkRequestFromEvent(*normalizedEvent)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
		request.RepositoryID = inbound.RepositoryID
		createdTask, err = handler.StartOrResumeMergeRequestRework(ctx, request)
		if err != nil {
			if errors.Is(err, model.ErrMergeRequestNotManaged) || errors.Is(err, model.ErrMergeRequestReworkRequesterUnauthorized) || errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrTaskNotClaimable) {
				return p.ignoreInboundEvent(ctx, inbound, err.Error(), duplicate)
			}
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
	} else if normalizedEvent.Kind == EventKindMergeRequestLifecycleChanged {
		handler, ok := p.taskCreator.(MergeRequestReworkHandler)
		if !ok {
			return nil, p.failInboundEvent(ctx, inbound, errors.New("merge request rework handler is required"))
		}
		request, err := mergeRequestLifecycleRequestFromEvent(*normalizedEvent)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
		createdTask, err = handler.ApplyMergeRequestLifecycle(ctx, request)
		if err != nil {
			if errors.Is(err, model.ErrMergeRequestNotManaged) || errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrTaskNotClaimable) {
				return p.ignoreInboundEvent(ctx, inbound, err.Error(), duplicate)
			}
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
	} else {
		task, err := p.taskFactory.BuildTask(*normalizedEvent)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
		task.RepositoryID = inbound.RepositoryID
		createdTask, err = p.taskCreator.CreateTask(ctx, task)
		if err != nil {
			return nil, p.failInboundEvent(ctx, inbound, err)
		}
	}

	inbound.Status = model.InboundEventStatusTaskCreated
	if err := p.events.Update(ctx, inbound); err != nil {
		return nil, err
	}

	return &ProcessResult{Event: inbound, Task: createdTask, Duplicate: duplicate}, nil
}

func mergeRequestLifecycleRequestFromEvent(event NormalizedEvent) (model.MergeRequestLifecycleRequest, error) {
	if strings.TrimSpace(event.DedupeKey) == "" || strings.TrimSpace(event.ProjectID) == "" || event.ObjectIID <= 0 || event.Metadata == nil {
		return model.MergeRequestLifecycleRequest{}, errors.New("merge request lifecycle identity and metadata are required")
	}
	state := metadataString(event.Metadata, "lifecycle_state")
	if state == "" {
		return model.MergeRequestLifecycleRequest{}, errors.New("merge request lifecycle state is required")
	}
	return model.MergeRequestLifecycleRequest{
		DedupeKey:       event.DedupeKey,
		ProjectID:       event.ProjectID,
		ProjectPath:     event.ProjectPath,
		MergeRequestIID: event.ObjectIID,
		Action:          event.EventAction,
		State:           state,
		HeadSHA:         event.HeadSHA,
	}, nil
}

func mergeRequestReworkRequestFromEvent(event NormalizedEvent) (model.MergeRequestReworkRequest, error) {
	if strings.TrimSpace(event.DedupeKey) == "" || event.Metadata == nil {
		return model.MergeRequestReworkRequest{}, errors.New("merge request rework identity and metadata are required")
	}
	noteID, ok := metadataPositiveInt(event.Metadata["note_id"])
	if !ok {
		return model.MergeRequestReworkRequest{}, errors.New("merge request rework note id is required")
	}
	requesterID, ok := metadataPositiveInt(event.Metadata["requester_id"])
	if !ok {
		return model.MergeRequestReworkRequest{}, errors.New("merge request rework requester id is required")
	}
	return model.MergeRequestReworkRequest{
		DedupeKey:               event.DedupeKey,
		ProjectID:               event.ProjectID,
		ProjectPath:             event.ProjectPath,
		RepositoryURL:           event.RepositoryURL,
		MergeRequestIID:         event.ObjectIID,
		MergeRequestURL:         event.ObjectURL,
		MergeRequestTitle:       event.Title,
		MergeRequestDescription: event.Description,
		SourceBranch:            event.SourceBranch,
		TargetBranch:            event.TargetBranch,
		ExpectedHeadSHA:         event.HeadSHA,
		NoteID:                  noteID,
		NoteURL:                 metadataString(event.Metadata, "note_url"),
		NoteType:                metadataString(event.Metadata, "note_type"),
		DiscussionID:            metadataString(event.Metadata, "discussion_id"),
		ResumeContext:           metadataString(event.Metadata, "resume_context"),
		RequesterID:             requesterID,
		RequesterName:           metadataString(event.Metadata, "requester_name"),
		RequesterUsername:       metadataString(event.Metadata, "requester_username"),
		RequesterEmail:          metadataString(event.Metadata, "requester_email"),
		RequesterPreAuthorized:  metadataBool(event.Metadata, "requester_pre_authorized"),
		AssigneeIDs:             gitLabAssigneeIDs(event.Metadata),
		ReviewerIDs:             metadataIntSlice(event.Metadata, "reviewer_ids"),
		Source:                  event.Source,
	}, nil
}

func metadataBool(metadata map[string]any, key string) bool {
	value, _ := metadata[key].(bool)
	return value
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func metadataIntSlice(metadata map[string]any, key string) []int {
	return gitLabAssigneeIDs(map[string]any{"assignee_ids": metadata[key]})
}

func (p *Processor) denyUnregisteredRepository(ctx context.Context, inbound *model.InboundEvent, event *NormalizedEvent, duplicate bool) (*ProcessResult, error) {
	if p.repos == nil || event == nil {
		return nil, nil
	}
	provider := registeredrepo.NormalizeProvider(event.Source)
	repo, err := p.repos.GetByProjectID(ctx, provider, event.ProjectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, repository.ErrNotFound) {
			// webhook secret 按仓验签后,真未注册的仓在验签处已被 401 拒绝、不会
			// 落库;此处 not-found 只剩「注册仓存在但平台身份未探测完成
			// (configuring)」的残余。enabled 仓必有 project id(启用前置探测),
			// 故并入 repository_not_enabled:可重放状态,启用补齐身份后同 UUID
			// 重放恢复评估。
			return p.deferInboundEvent(ctx, inbound, "repository_not_enabled", duplicate)
		}
		return nil, err
	}
	repositoryID := repo.RepositoryID
	inbound.RepositoryID = &repositoryID
	if err := p.events.Update(ctx, inbound); err != nil {
		return nil, err
	}
	if strings.TrimSpace(repo.PathWithNamespace) != strings.TrimSpace(event.ProjectPath) {
		return p.ignoreInboundEvent(ctx, inbound, "repository_identity_mismatch", duplicate)
	}
	if repo.Status != registeredrepo.StatusEnabled {
		// 仓库已注册但未启用（含 token 失效降级 configuring 窗口）：可重放状态，
		// 启用后同 UUID 重放会重新评估并创建任务。
		return p.deferInboundEvent(ctx, inbound, "repository_not_enabled", duplicate)
	}
	return nil, nil
}

func (p *Processor) deferInboundEvent(ctx context.Context, inbound *model.InboundEvent, reason string, duplicate bool) (*ProcessResult, error) {
	inbound.Status = model.InboundEventStatusDeferred
	inbound.ErrorMessage = reason
	if err := p.events.Update(ctx, inbound); err != nil {
		return nil, err
	}
	return &ProcessResult{Event: inbound, Ignored: false, IgnoreReason: reason, Duplicate: duplicate}, nil
}

func (p *Processor) ignoreInboundEvent(ctx context.Context, inbound *model.InboundEvent, reason string, duplicate bool) (*ProcessResult, error) {
	inbound.Status = model.InboundEventStatusIgnored
	inbound.ErrorMessage = reason
	if err := p.events.Update(ctx, inbound); err != nil {
		return nil, err
	}
	return &ProcessResult{Event: inbound, Ignored: true, IgnoreReason: reason, Duplicate: duplicate}, nil
}

func developerResumeRequestFromEvent(event NormalizedEvent) (model.DeveloperResumeRequest, error) {
	if strings.TrimSpace(event.DedupeKey) == "" {
		return model.DeveloperResumeRequest{}, errors.New("developer resume dedupe key is required")
	}
	if event.Metadata == nil {
		return model.DeveloperResumeRequest{}, errors.New("developer resume metadata is required")
	}

	commentID, ok := metadataPositiveInt(event.Metadata["comment_id"])
	if !ok {
		return model.DeveloperResumeRequest{}, errors.New("developer resume note id is required")
	}
	resumeContext, ok := event.Metadata["resume_context"].(string)
	if !ok || strings.TrimSpace(resumeContext) == "" {
		return model.DeveloperResumeRequest{}, errors.New("developer resume context is required")
	}

	return model.DeveloperResumeRequest{
		DedupeKey:     event.DedupeKey,
		NoteID:        commentID,
		ResumeContext: strings.TrimSpace(resumeContext),
		AssigneeIDs:   gitLabAssigneeIDs(event.Metadata),
	}, nil
}

func metadataPositiveInt(value any) (int, bool) {
	switch id := value.(type) {
	case int:
		return id, id > 0
	case float64:
		converted := int(id)
		return converted, id > 0 && id == float64(converted)
	default:
		return 0, false
	}
}

type gitLabReceipt struct {
	DedupeKey   string
	EventType   string
	EventAction string
	ExternalID  string
}

func newGitLabReceipt(raw []byte, metadata GitLabWebhookMetadata) gitLabReceipt {
	var payload gitLabWebhookPayload
	_ = json.Unmarshal(raw, &payload)

	eventType := eventTypeOrDefault(payload.EventType, payload.ObjectKind)
	if strings.TrimSpace(eventType) == "" {
		eventType = "unknown"
	}
	action := strings.TrimSpace(payload.ObjectAttributes.Action)
	if action == "" {
		action = "unknown"
	}
	projectID := strconv.Itoa(payload.Project.ID)
	if payload.Project.ID <= 0 {
		projectID = "unknown"
	}

	objectIID := payload.ObjectAttributes.IID
	externalID := fmt.Sprintf("%s:%d", projectID, objectIID)
	dedupeKey := gitLabReceiptDedupeKey(raw, eventType, projectID, objectIID, action, metadata)

	return gitLabReceipt{
		DedupeKey:   dedupeKey,
		EventType:   eventType,
		EventAction: action,
		ExternalID:  externalID,
	}
}

func gitLabReceiptDedupeKey(raw []byte, eventType, projectID string, objectIID int, action string, metadata GitLabWebhookMetadata) string {
	if eventUUID := strings.TrimSpace(metadata.EventUUID); eventUUID != "" {
		return fmt.Sprintf("gitlab:webhook:event_uuid:%s", eventUUID)
	}
	rawHash := shortRawSHA256(raw)
	return fmt.Sprintf("gitlab:webhook:%s:%s:%d:%s:%s", eventType, projectID, objectIID, action, rawHash)
}

func shortRawSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	encoded := hex.EncodeToString(sum[:])
	return encoded[:16]
}

func (p *Processor) failInboundEvent(ctx context.Context, inbound *model.InboundEvent, cause error) error {
	inbound.Status = model.InboundEventStatusFailed
	inbound.ErrorMessage = cause.Error()
	if err := p.events.Update(ctx, inbound); err != nil {
		return err
	}
	return cause
}
