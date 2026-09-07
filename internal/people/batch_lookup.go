package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const batchLookupConcurrency = 4

type BatchLookupRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Token    string `json:"-"`
}

type BatchLookupItem struct {
	PersonID        string            `json:"person_id"`
	DisplayName     string            `json:"display_name"`
	Email           string            `json:"email"`
	IdentityID      string            `json:"identity_id"`
	Candidates      []LookupCandidate `json:"candidates"`
	ExactMatchIndex *int              `json:"exact_match_index"`
	ErrorCode       string            `json:"error_code,omitempty"`
}

type BatchLookupResult struct {
	Items []BatchLookupItem `json:"items"`
}

type BatchLookupService struct {
	directory *Service
	lookup    IdentityLookup
}

func NewBatchLookupService(directory *Service, lookup IdentityLookup) *BatchLookupService {
	return &BatchLookupService{directory: directory, lookup: lookup}
}

// supportedBatchLookupProviders 是批量补全支持的平台;具体查询能力由装配的
// IdentityLookup 客户端决定(GitLab 按邮箱前缀搜索,Gitea/GitHub 按用户名精确查)。
var supportedBatchLookupProviders = map[string]struct{}{
	"gitlab": {}, "gitea": {}, "github": {},
}

func (service *BatchLookupService) Lookup(ctx context.Context, request BatchLookupRequest) (BatchLookupResult, error) {
	provider := strings.ToLower(strings.TrimSpace(request.Provider))
	if _, ok := supportedBatchLookupProviders[provider]; !ok || service == nil || service.directory == nil || service.lookup == nil {
		return BatchLookupResult{}, fmt.Errorf("%w: batch lookup is unavailable for provider %q", ErrInvalidBatchLookup, provider)
	}
	baseURL, err := normalizeBaseURL(request.BaseURL)
	if err != nil || strings.TrimSpace(request.Token) == "" {
		return BatchLookupResult{}, fmt.Errorf("%w: base_url and token are required", ErrInvalidBatchLookup)
	}

	items, err := service.incompleteIdentities(ctx, provider, baseURL)
	if err != nil || len(items) == 0 {
		return BatchLookupResult{Items: items}, err
	}

	lookupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var workers sync.WaitGroup
	var fatalMu sync.Mutex
	var fatalErr error
	workerCount := batchLookupConcurrency
	if len(items) < workerCount {
		workerCount = len(items)
	}
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				result, lookupErr := service.lookup.Lookup(lookupCtx, LookupRequest{
					Provider: provider, BaseURL: baseURL, Token: strings.TrimSpace(request.Token), Email: items[index].Email,
				})
				if lookupErr == nil {
					items[index].Candidates = result.Candidates
					items[index].ExactMatchIndex = result.ExactMatchIndex
					continue
				}
				if isFatalBatchLookupError(provider, lookupErr) {
					fatalMu.Lock()
					if fatalErr == nil || errors.Is(lookupErr, ErrGitLabAuthFailed) {
						fatalErr = lookupErr
					}
					fatalMu.Unlock()
					cancel()
					continue
				}
				items[index].ErrorCode = "lookup_failed"
			}
		}()
	}
	func() {
		defer close(jobs)
		for index := range items {
			select {
			case <-lookupCtx.Done():
				return
			case jobs <- index:
			}
		}
	}()
	workers.Wait()
	fatalMu.Lock()
	defer fatalMu.Unlock()
	if fatalErr != nil {
		return BatchLookupResult{}, fatalErr
	}
	return BatchLookupResult{Items: items}, nil
}

func (service *BatchLookupService) incompleteIdentities(ctx context.Context, provider, baseURL string) ([]BatchLookupItem, error) {
	items := make([]BatchLookupItem, 0)
	for pageNumber := 1; ; pageNumber++ {
		page, err := service.directory.List(ctx, PersonFilter{
			PageRequest: PageRequest{Page: pageNumber, PageSize: maximumPageSize}, Provider: provider, BaseURL: baseURL,
		})
		if err != nil {
			return nil, err
		}
		for _, person := range page.Items {
			for _, identity := range person.Identities {
				if identity.Provider != provider || identity.CodeHostBaseURL != baseURL || (identity.Username != "" && identity.ExternalUserID != "") {
					continue
				}
				items = append(items, BatchLookupItem{
					PersonID: person.PersonID, DisplayName: person.DisplayName, Email: person.Email,
					IdentityID: identity.IdentityID, Candidates: []LookupCandidate{},
				})
			}
		}
		if pageNumber*page.PageSize >= int(page.Total) || len(page.Items) == 0 {
			break
		}
	}
	return items, nil
}

// isFatalBatchLookupError 判定单条查询错误是否致命(整批终止):鉴权失败与
// 平台不可达会 equally 破坏整轮补全;单条未命中/其他错误只标注在条目上。
func isFatalBatchLookupError(provider string, err error) bool {
	switch provider {
	case "gitlab":
		return errors.Is(err, ErrGitLabAuthFailed) || errors.Is(err, ErrGitLabUnavailable)
	case "gitea":
		return errors.Is(err, ErrGiteaAuthFailed) || errors.Is(err, ErrGiteaUnavailable)
	case "github":
		return errors.Is(err, ErrGitHubAuthFailed) || errors.Is(err, ErrGitHubUnavailable)
	}
	return false
}
