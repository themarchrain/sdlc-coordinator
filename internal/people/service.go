package people

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"sdlc-coordinator/internal/store"

	"gorm.io/gorm"
)

var (
	ErrInvalidBatchLookup       = errors.New("invalid batch lookup request")
	ErrGiteaAuthFailed          = errors.New("Gitea authentication failed")
	ErrGitHubAuthFailed         = errors.New("GitHub authentication failed")
	ErrInvalidPerson            = errors.New("invalid person")
	ErrIncompleteEnabledPerson  = errors.New("enabled person has incomplete codehost identity")
	ErrPersonEmailConflict      = errors.New("person email conflict")
	ErrCodeHostIdentityConflict = errors.New("codehost identity conflict")
	ErrNotFound                 = errors.New("person not found")
)

type IdentityInput struct {
	IdentityID     string
	Provider       string
	BaseURL        string
	ExternalUserID string
	Username       string
}

type CreatePersonRequest struct {
	DisplayName string
	Email       string
	Enabled     bool
	Identities  []IdentityInput
}

type UpdatePersonRequest = CreatePersonRequest

type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }

// MentionIdentity 是 IM 提及渲染所需的人员解析结果。
type MentionIdentity struct {
	DisplayName string // 人员显示名;缺席时回退平台 username
	Username    string // 平台用户名
}

// ResolveMentionNames 把平台用户 ID 解析为提及材料(显示名,兜底平台 username)。
// 目录缺席或身份未命中时条目为空——通知链的优雅降级由调用方完成,这里不报错。
func (service *Service) ResolveMentionNames(ctx context.Context, provider, baseURL string, assigneeIDs []int) map[int]MentionIdentity {
	result := make(map[int]MentionIdentity, len(assigneeIDs))
	if service == nil || service.repository == nil || len(assigneeIDs) == 0 {
		return result
	}
	ids := make([]string, 0, len(assigneeIDs))
	index := make(map[string]int, len(assigneeIDs))
	for _, id := range assigneeIDs {
		if id <= 0 {
			continue
		}
		key := strconv.Itoa(id)
		if _, exists := index[key]; exists {
			continue
		}
		ids = append(ids, key)
		index[key] = id
	}
	owners, err := service.repository.ListIdentityOwners(ctx, provider, baseURL, ids)
	if err != nil || len(owners) == 0 {
		return result
	}
	for _, owner := range owners {
		displayName := strings.TrimSpace(owner.Person.DisplayName)
		username := strings.TrimSpace(owner.Identity.Username)
		if displayName == "" {
			displayName = username
		}
		result[index[owner.Identity.ExternalUserID]] = MentionIdentity{DisplayName: displayName, Username: username}
	}
	return result
}

func (service *Service) Create(ctx context.Context, request CreatePersonRequest) (*Person, error) {
	person, identities, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	if err := service.repository.Create(ctx, person, identities); err != nil {
		return nil, translateConflict(err)
	}
	return person, nil
}

func (service *Service) Get(ctx context.Context, personID string) (*Person, error) {
	return service.repository.Get(ctx, personID)
}

func (service *Service) List(ctx context.Context, filter PersonFilter) (Page[Person], error) {
	filter.Provider = strings.ToLower(strings.TrimSpace(filter.Provider))
	if filter.BaseURL != "" {
		normalized, err := normalizeBaseURL(filter.BaseURL)
		if err != nil {
			return Page[Person]{}, err
		}
		filter.BaseURL = normalized
	}
	return service.repository.List(ctx, filter)
}

func (service *Service) Update(ctx context.Context, personID string, request UpdatePersonRequest) (*Person, error) {
	person, identities, err := normalizeRequest(CreatePersonRequest(request))
	if err != nil {
		return nil, err
	}
	person.PersonID = strings.TrimSpace(personID)
	if person.PersonID == "" {
		return nil, fmt.Errorf("%w: person_id is required", ErrInvalidPerson)
	}
	if err := service.repository.Replace(ctx, person, identities); err != nil {
		return nil, translateConflict(err)
	}
	return service.repository.Get(ctx, person.PersonID)
}

func (service *Service) Delete(ctx context.Context, personID string) error {
	return service.repository.Delete(ctx, personID)
}

func normalizeRequest(request CreatePersonRequest) (*Person, []CodeHostIdentity, error) {
	name := strings.TrimSpace(request.DisplayName)
	if name == "" {
		return nil, nil, fmt.Errorf("%w: display_name is required", ErrInvalidPerson)
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email {
		return nil, nil, fmt.Errorf("%w: email is invalid", ErrInvalidPerson)
	}
	identities := make([]CodeHostIdentity, 0, len(request.Identities))
	for _, input := range request.Identities {
		identity, err := normalizeIdentity(input)
		if err != nil {
			return nil, nil, err
		}
		identities = append(identities, identity)
	}
	if request.Enabled {
		if len(identities) == 0 {
			return nil, nil, fmt.Errorf("%w: at least one codehost identity is required", ErrIncompleteEnabledPerson)
		}
		for _, identity := range identities {
			if identity.ExternalUserID == "" || identity.Username == "" {
				return nil, nil, fmt.Errorf("%w: username and external_user_id are required", ErrIncompleteEnabledPerson)
			}
		}
	}
	return &Person{DisplayName: name, Email: email, Enabled: request.Enabled}, identities, nil
}

func normalizeIdentity(input IdentityInput) (CodeHostIdentity, error) {
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	if provider != "gitlab" && provider != "gitea" && provider != "github" {
		return CodeHostIdentity{}, fmt.Errorf("%w: provider must be gitlab, gitea or github", ErrInvalidPerson)
	}
	baseURL, err := normalizeBaseURL(input.BaseURL)
	if err != nil {
		return CodeHostIdentity{}, err
	}
	externalUserID := strings.TrimSpace(input.ExternalUserID)
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if (externalUserID == "") != (username == "") {
		return CodeHostIdentity{}, fmt.Errorf("%w: external_user_id and username must be filled together", ErrInvalidPerson)
	}
	return CodeHostIdentity{
		IdentityID: strings.TrimSpace(input.IdentityID), Provider: provider, CodeHostBaseURL: baseURL,
		ExternalUserID: externalUserID, Username: username,
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: base_url must be an http or https origin", ErrInvalidPerson)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func translateConflict(err error) error {
	if !isUniqueViolation(err) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "people.email") || strings.Contains(message, "uidx_people_email") {
		return ErrPersonEmailConflict
	}
	if strings.Contains(message, "codehost_identities") || strings.Contains(message, "uidx_identity") {
		return ErrCodeHostIdentityConflict
	}
	return ErrCodeHostIdentityConflict
}

func isUniqueViolation(err error) bool {
	// 驱动错误码分类（MySQL 1062 / SQLite 2067+1555），不依赖错误文本；
	// gorm.ErrDuplicatedKey 只在开启 TranslateError 时出现，兼容保留。
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return store.IsDuplicateKey(err)
}
