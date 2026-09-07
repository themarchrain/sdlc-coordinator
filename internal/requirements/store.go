package requirements

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Migrate() error {
	if err := s.db.AutoMigrate(&Requirement{}); err != nil {
		return fmt.Errorf("migrate requirements: %w", err)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, item *Requirement) error {
	if item == nil {
		return fmt.Errorf("requirement is required")
	}
	if item.Status != "" && !ValidStatus(item.Status) {
		return fmt.Errorf("invalid requirement status")
	}
	if err := s.db.WithContext(ctx).Create(item).Error; err != nil {
		return fmt.Errorf("create requirement: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, item *Requirement) error {
	if item == nil {
		return fmt.Errorf("requirement is required")
	}
	if err := s.db.WithContext(ctx).Save(item).Error; err != nil {
		return fmt.Errorf("update requirement: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, requirementID string) (*Requirement, error) {
	var item Requirement
	if err := s.db.WithContext(ctx).Where("requirement_id = ?", strings.TrimSpace(requirementID)).First(&item).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("get requirement: %w", err)
	}
	return &item, nil
}

type ListFilter struct {
	Status       Status
	RepositoryID string
	Page         int
	PageSize     int
}

func (s *Store) List(ctx context.Context, filter ListFilter) ([]Requirement, int64, error) {
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := s.db.WithContext(ctx).Model(&Requirement{})
	if filter.Status != "" {
		if !ValidStatus(filter.Status) {
			return nil, 0, fmt.Errorf("invalid status filter")
		}
		query = query.Where("status = ?", filter.Status)
	}
	if repositoryID := strings.TrimSpace(filter.RepositoryID); repositoryID != "" {
		query = query.Where("repository_id = ?", repositoryID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count requirements: %w", err)
	}
	var items []Requirement
	if err := query.Order("created_at DESC").Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list requirements: %w", err)
	}
	return items, total, nil
}

func (s *Store) ListProcessable(ctx context.Context, statuses []Status, limit int) ([]Requirement, error) {
	if len(statuses) == 0 || limit <= 0 {
		return nil, nil
	}
	var items []Requirement
	if err := s.db.WithContext(ctx).Where("status IN ?", statuses).Order("created_at ASC").Order("id ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list processable requirements: %w", err)
	}
	return items, nil
}
