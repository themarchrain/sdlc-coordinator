package requirements

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/adapters/codehost"
	"sdlc-coordinator/internal/registeredrepo"

	"gorm.io/gorm"
)

const (
	// 上限只绑定写操作;镜像为读操作,独立限额(候选饥饿教训:读检查不消耗写额度)。
	createLimitPerRound = 5
	mirrorLimitPerRound = 25
)

type SyncResult struct {
	Created      int `json:"created"`
	Recovered    int `json:"recovered"`
	Failed       int `json:"failed"`
	Skipped      int `json:"skipped"`
	Mirrored     int `json:"mirrored"`
	MirrorErrors int `json:"mirror_errors"`
}

func (s *Service) SyncOnce(ctx context.Context) (SyncResult, error) {
	if !s.syncMu.TryLock() {
		return SyncResult{}, ErrSyncInProgress
	}
	defer s.syncMu.Unlock()
	var result SyncResult
	created, recovered, failed, skipped, err := s.publishProcessableRequirements(ctx)
	if err != nil {
		return result, err
	}
	result.Created, result.Recovered, result.Failed, result.Skipped = created, recovered, failed, skipped
	mirrored, mirrorErrors, err := s.mirrorPublishedRequirements(ctx)
	if err != nil {
		return result, err
	}
	result.Mirrored, result.MirrorErrors = mirrored, mirrorErrors
	return result, nil
}

func (s *Service) publishProcessableRequirements(ctx context.Context) (created, recovered, failed, skipped int, err error) {
	items, err := s.store.ListProcessable(ctx, []Status{StatusQueued, StatusFailed}, createLimitPerRound)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	for i := range items {
		outcome, itemErr := s.publishRequirement(ctx, &items[i])
		if itemErr != nil {
			return created, recovered, failed, skipped, itemErr
		}
		switch outcome {
		case publishCreated:
			created++
		case publishRecovered:
			recovered++
		case publishFailed:
			failed++
		case publishSkipped:
			skipped++
		}
	}
	return created, recovered, failed, skipped, nil
}

type publishOutcome int

const (
	publishCreated publishOutcome = iota
	publishRecovered
	publishFailed
	publishSkipped
)

func (s *Service) publishRequirement(ctx context.Context, item *Requirement) (publishOutcome, error) {
	repo, err := s.repositories.Get(ctx, item.RepositoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return publishFailed, s.markFailure(ctx, item, "repository no longer exists")
		}
		return publishFailed, err
	}
	if repo.Status != registeredrepo.StatusEnabled {
		// 禁用可能是临时的:保持 queued,只写提示,不置 failed。
		item.LastError = "repository is not enabled"
		return publishSkipped, s.store.Update(ctx, item)
	}
	if s.clients == nil {
		return publishFailed, s.markFailure(ctx, item, "codehost client factory is required")
	}
	client := s.clients(repo)
	projectID := ResolveProjectID(repo)

	// 崩溃恢复:上一轮"创建成功但本地回写中断"的孤儿 issue 靠描述内嵌签名回查绑定,绝不重复创建。
	found, err := client.ReconcileWriteback(ctx, codehost.ReconcileRequest{
		Kind:            "create_issue",
		ProjectID:       projectID,
		DedupeKey:       item.SourceKey(),
		OperationMarker: item.SourceKey(),
	})
	if err != nil {
		return publishFailed, s.markFailure(ctx, item, err.Error())
	}
	if found.State == codehost.ReconcileStateFound {
		iid, parseErr := strconv.Atoi(strings.TrimSpace(found.ExternalID))
		if parseErr != nil {
			return publishFailed, s.markFailure(ctx, item, "recovered issue iid is not numeric")
		}
		item.IssueIID = iid
		item.IssueURL = found.ExternalURL
		markPublished(item)
		return publishRecovered, s.store.Update(ctx, item)
	}

	issue, err := client.CreateIssue(ctx, projectID, codehost.CreateIssueRequest{
		Title:       item.Title,
		Description: RenderDescription(item.Description, item.SourceKey()),
		Labels:      MergeDeveloperAgentLabel(item.LabelList()),
	})
	if err != nil {
		return publishFailed, s.markFailure(ctx, item, err.Error())
	}
	item.IssueIID = issue.IID
	item.IssueURL = issue.WebURL
	markPublished(item)
	return publishCreated, s.store.Update(ctx, item)
}

func markPublished(item *Requirement) {
	item.Status = StatusPublished
	item.LastError = ""
	now := time.Now()
	item.PublishedAt = &now
}

func (s *Service) markFailure(ctx context.Context, item *Requirement, message string) error {
	item.Status = StatusFailed
	item.LastError = truncateMessage(message, 500)
	return s.store.Update(ctx, item)
}

func (s *Service) mirrorPublishedRequirements(ctx context.Context) (mirrored, mirrorErrors int, err error) {
	items, err := s.store.ListProcessable(ctx, []Status{StatusPublished}, mirrorLimitPerRound)
	if err != nil {
		return 0, 0, err
	}
	for i := range items {
		item := &items[i]
		if item.IssueIID <= 0 {
			mirrorErrors++
			continue
		}
		repo, repoErr := s.repositories.Get(ctx, item.RepositoryID)
		if repoErr != nil || repo.Status != registeredrepo.StatusEnabled {
			mirrorErrors++
			continue
		}
		if s.clients == nil {
			mirrorErrors++
			continue
		}
		client := s.clients(repo)
		state, stateErr := client.GetIssueState(ctx, ResolveProjectID(repo), item.IssueIID)
		if stateErr != nil {
			mirrorErrors++
			continue
		}
		if !state.Closed {
			continue
		}
		item.Status = StatusDone
		now := time.Now()
		item.CompletedAt = &now
		if updateErr := s.store.Update(ctx, item); updateErr != nil {
			return mirrored, mirrorErrors, updateErr
		}
		mirrored++
	}
	return mirrored, mirrorErrors, nil
}

func truncateMessage(message string, limit int) string {
	runes := []rune(strings.TrimSpace(message))
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return string(runes)
}
