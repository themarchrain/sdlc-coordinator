package connectors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	storerepository "sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

const (
	defaultPageSize = 20
	maximumPageSize = 100
)

type PageRequest struct {
	Page         int
	PageSize     int
	RepositoryID string
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

type ConnectorFilter struct {
	PageRequest
	Kind              string
	Enabled           *bool
	TargetProjectPath string
}

type ConnectorRepository struct {
	db *gorm.DB
}

type HealthSnapshot struct {
	Status           model.ConnectorHealthStatus
	Message          string
	At               time.Time
	CapabilitiesJSON *string
}

func NewConnectorRepository(db *gorm.DB) *ConnectorRepository {
	return &ConnectorRepository{db: db}
}

func (repository *ConnectorRepository) Create(ctx context.Context, connector *model.ConnectorInstance) error {
	if err := repository.db.WithContext(ctx).Create(connector).Error; err != nil {
		return fmt.Errorf("create connector: %w", err)
	}
	return nil
}

func (repository *ConnectorRepository) Get(ctx context.Context, connectorID string) (*model.ConnectorInstance, error) {
	var connector model.ConnectorInstance
	if err := repository.db.WithContext(ctx).
		Where("connector_id = ?", strings.TrimSpace(connectorID)).
		First(&connector).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, storerepository.ErrNotFound
		}
		return nil, fmt.Errorf("get connector: %w", err)
	}
	return &connector, nil
}

func (repository *ConnectorRepository) List(ctx context.Context, filter ConnectorFilter) (Page[model.ConnectorInstance], error) {
	pageRequest := filter.PageRequest.normalize()
	query := repository.db.WithContext(ctx).Model(&model.ConnectorInstance{})
	if value := strings.TrimSpace(filter.Kind); value != "" {
		query = query.Where("kind = ?", value)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}
	if value := strings.TrimSpace(filter.TargetProjectPath); value != "" {
		query = query.Where("target_project_path = ?", value)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return Page[model.ConnectorInstance]{}, fmt.Errorf("count connectors: %w", err)
	}
	items := make([]model.ConnectorInstance, 0)
	if err := query.
		Order("created_at DESC").
		Order("id DESC").
		Offset((pageRequest.Page - 1) * pageRequest.PageSize).
		Limit(pageRequest.PageSize).
		Find(&items).Error; err != nil {
		return Page[model.ConnectorInstance]{}, fmt.Errorf("list connectors: %w", err)
	}
	return Page[model.ConnectorInstance]{
		Items: items, Page: pageRequest.Page, PageSize: pageRequest.PageSize, Total: total,
	}, nil
}

func (repository *ConnectorRepository) Update(ctx context.Context, connector *model.ConnectorInstance) error {
	result := repository.db.WithContext(ctx).Model(&model.ConnectorInstance{}).
		Where("connector_id = ?", strings.TrimSpace(connector.ConnectorID)).
		Updates(map[string]any{
			"kind": connector.Kind, "display_name": connector.DisplayName, "base_url": connector.BaseURL,
			"enabled": connector.Enabled, "target_project_path": connector.TargetProjectPath, "updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update connector: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return storerepository.ErrNotFound
	}
	return nil
}

func (repository *ConnectorRepository) UpdateHealthSnapshot(ctx context.Context, connectorID, expectedBaseURL string, snapshot HealthSnapshot) error {
	updates := map[string]any{
		"last_health_status":  snapshot.Status,
		"last_health_message": snapshot.Message,
		"last_health_at":      snapshot.At,
		"updated_at":          snapshot.At,
	}
	if snapshot.CapabilitiesJSON != nil {
		updates["capabilities_json"] = *snapshot.CapabilitiesJSON
	}
	result := repository.db.WithContext(ctx).Model(&model.ConnectorInstance{}).
		Where("connector_id = ? AND enabled = ? AND base_url = ?", strings.TrimSpace(connectorID), true, strings.TrimSpace(expectedBaseURL)).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update connector health: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	current, err := repository.Get(ctx, connectorID)
	if err != nil {
		return err
	}
	if !current.Enabled {
		return ErrConnectorDisabled
	}
	return ErrConnectorChanged
}

func (repository *ConnectorRepository) Delete(ctx context.Context, connectorID string) error {
	result := repository.db.WithContext(ctx).
		Where("connector_id = ?", strings.TrimSpace(connectorID)).
		Delete(&model.ConnectorInstance{})
	if result.Error != nil {
		return fmt.Errorf("delete connector: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return storerepository.ErrNotFound
	}
	return nil
}
