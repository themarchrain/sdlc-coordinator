package people

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultPageSize = 20
	maximumPageSize = 100
)

type PageRequest struct {
	Page     int
	PageSize int
}

func (request PageRequest) normalize() PageRequest {
	if request.Page < 1 {
		request.Page = 1
	}
	if request.PageSize < 1 {
		request.PageSize = defaultPageSize
	}
	if request.PageSize > maximumPageSize {
		request.PageSize = maximumPageSize
	}
	return request
}

type Page[T any] struct {
	Items    []T
	Page     int
	PageSize int
	Total    int64
}

type PersonFilter struct {
	PageRequest
	Query    string
	Enabled  *bool
	Provider string
	BaseURL  string
}

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (repository *Repository) Migrate() error {
	if err := repository.db.AutoMigrate(&Person{}, &CodeHostIdentity{}); err != nil {
		return fmt.Errorf("migrate people directory: %w", err)
	}
	if repository.db.Migrator().HasIndex(&CodeHostIdentity{}, "idx_codehost_identities_identity_id") {
		if err := repository.db.Migrator().DropIndex(&CodeHostIdentity{}, "idx_codehost_identities_identity_id"); err != nil {
			return fmt.Errorf("drop legacy identity id index: %w", err)
		}
	}
	for _, indexName := range []string{"uidx_identity_external", "uidx_identity_username"} {
		if err := repository.db.Exec("DROP INDEX IF EXISTS " + indexName).Error; err != nil {
			return fmt.Errorf("drop legacy %s index: %w", indexName, err)
		}
	}
	identityIndexes := []string{
		"CREATE UNIQUE INDEX uidx_identity_external ON codehost_identities(provider, code_host_base_url, external_user_id) WHERE deleted_at IS NULL AND external_user_id <> ''",
		"CREATE UNIQUE INDEX uidx_identity_username ON codehost_identities(provider, code_host_base_url, username) WHERE deleted_at IS NULL AND username <> ''",
	}
	for _, statement := range identityIndexes {
		if err := repository.db.Exec(statement).Error; err != nil {
			return fmt.Errorf("create draft-aware identity index: %w", err)
		}
	}
	return nil
}

func (repository *Repository) Create(ctx context.Context, person *Person, identities []CodeHostIdentity) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(person).Error; err != nil {
			return fmt.Errorf("create person: %w", err)
		}
		for index := range identities {
			identities[index].PersonID = person.PersonID
		}
		if len(identities) > 0 {
			if err := tx.Create(&identities).Error; err != nil {
				return fmt.Errorf("create codehost identities: %w", err)
			}
		}
		person.Identities = identities
		return nil
	})
}

func (repository *Repository) Get(ctx context.Context, personID string) (*Person, error) {
	var person Person
	if err := repository.db.WithContext(ctx).Preload("Identities").Where("person_id = ?", strings.TrimSpace(personID)).First(&person).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get person: %w", err)
	}
	return &person, nil
}

func (repository *Repository) List(ctx context.Context, filter PersonFilter) (Page[Person], error) {
	page := filter.PageRequest.normalize()
	query := repository.db.WithContext(ctx).Model(&Person{})
	if value := strings.TrimSpace(filter.Query); value != "" {
		needle := "%" + strings.ToLower(value) + "%"
		query = query.Where("LOWER(people.display_name) LIKE ? OR LOWER(people.email) LIKE ? OR EXISTS (SELECT 1 FROM codehost_identities WHERE codehost_identities.person_id = people.person_id AND codehost_identities.deleted_at IS NULL AND LOWER(codehost_identities.username) LIKE ?)", needle, needle, needle)
	}
	if filter.Enabled != nil {
		query = query.Where("people.enabled = ?", *filter.Enabled)
	}
	provider := strings.TrimSpace(filter.Provider)
	baseURL := strings.TrimSpace(filter.BaseURL)
	if provider != "" || baseURL != "" {
		identityClause := "EXISTS (SELECT 1 FROM codehost_identities WHERE codehost_identities.person_id = people.person_id AND codehost_identities.deleted_at IS NULL"
		identityArgs := make([]any, 0, 2)
		if provider != "" {
			identityClause += " AND codehost_identities.provider = ?"
			identityArgs = append(identityArgs, provider)
		}
		if baseURL != "" {
			identityClause += " AND codehost_identities.code_host_base_url = ?"
			identityArgs = append(identityArgs, baseURL)
		}
		query = query.Where(identityClause+")", identityArgs...)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return Page[Person]{}, fmt.Errorf("count people: %w", err)
	}
	items := make([]Person, 0)
	if err := query.Preload("Identities").Order("people.updated_at DESC").Order("people.id DESC").Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize).Find(&items).Error; err != nil {
		return Page[Person]{}, fmt.Errorf("list people: %w", err)
	}
	return Page[Person]{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, nil
}

func (repository *Repository) Replace(ctx context.Context, person *Person, identities []CodeHostIdentity) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Person{}).Where("person_id = ?", person.PersonID).Updates(map[string]any{
			"display_name": person.DisplayName, "email": person.Email, "enabled": person.Enabled, "updated_at": time.Now().UTC(),
		})
		if result.Error != nil {
			return fmt.Errorf("update person: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		if err := tx.Where("person_id = ?", person.PersonID).Delete(&CodeHostIdentity{}).Error; err != nil {
			return fmt.Errorf("delete prior codehost identities: %w", err)
		}
		for index := range identities {
			identities[index].PersonID = person.PersonID
		}
		if len(identities) > 0 {
			if err := tx.Create(&identities).Error; err != nil {
				return fmt.Errorf("replace codehost identities: %w", err)
			}
		}
		person.Identities = identities
		return nil
	})
}

// IdentityOwner 是身份及其归属人员的关联行,供提及解析按平台身份反查人员。
type IdentityOwner struct {
	Identity CodeHostIdentity
	Person   Person
}

// ListIdentityOwners 按 (provider, base_url, external_user_id) 反查身份与归属人;
// 软删除的身份/人员天然被排除,未命中的 ID 不在结果里,调用方据此降级。
func (repository *Repository) ListIdentityOwners(ctx context.Context, provider, baseURL string, externalIDs []string) ([]IdentityOwner, error) {
	provider = strings.TrimSpace(provider)
	baseURL = strings.TrimSpace(baseURL)
	ids := make([]string, 0, len(externalIDs))
	seen := make(map[string]struct{}, len(externalIDs))
	for _, id := range externalIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if provider == "" || baseURL == "" || len(ids) == 0 {
		return nil, nil
	}
	var identities []CodeHostIdentity
	if err := repository.db.WithContext(ctx).
		Where("provider = ? AND code_host_base_url = ? AND external_user_id IN ?", provider, baseURL, ids).
		Find(&identities).Error; err != nil {
		return nil, fmt.Errorf("list identity owners: %w", err)
	}
	owners := make([]IdentityOwner, 0, len(identities))
	for _, identity := range identities {
		var person Person
		if err := repository.db.WithContext(ctx).Where("person_id = ?", identity.PersonID).First(&person).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, fmt.Errorf("get identity owner: %w", err)
		}
		owners = append(owners, IdentityOwner{Identity: identity, Person: person})
	}
	return owners, nil
}

func (repository *Repository) Delete(ctx context.Context, personID string) error {
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("person_id = ?", strings.TrimSpace(personID)).Delete(&CodeHostIdentity{}).Error; err != nil {
			return fmt.Errorf("delete codehost identities: %w", err)
		}
		result := tx.Where("person_id = ?", strings.TrimSpace(personID)).Delete(&Person{})
		if result.Error != nil {
			return fmt.Errorf("delete person: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			var existing int64
			if err := tx.Unscoped().Model(&Person{}).Where("person_id = ?", strings.TrimSpace(personID)).Count(&existing).Error; err != nil {
				return fmt.Errorf("check deleted person: %w", err)
			}
			if existing > 0 {
				return nil
			}
			return ErrNotFound
		}
		return nil
	})
}
