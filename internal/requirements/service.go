package requirements

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"sdlc-coordinator/internal/registeredrepo"

	"gorm.io/gorm"
)

var (
	ErrNotFound             = errors.New("requirement not found")
	ErrInvalidRequirement   = errors.New("invalid requirement")
	ErrRepositoryNotEnabled = errors.New("requirement repository is not enabled")
	ErrNotEditable          = errors.New("requirement is not editable")
	ErrInvalidTransition    = errors.New("invalid requirement status transition")
	ErrSyncInProgress       = errors.New("requirements sync already running")
)

type CreateRequest struct {
	Title        string
	Description  string
	RepositoryID string
	Labels       []string
	Priority     string
}

type PatchRequest struct {
	Title       *string
	Description *string
	Labels      *[]string
	Priority    *string
}

type Service struct {
	store        *Store
	repositories *registeredrepo.Store
	clients      ClientFactoryFunc
	syncMu       sync.Mutex
}

func NewService(store *Store, repositories *registeredrepo.Store, clients ClientFactoryFunc) *Service {
	return &Service{
		store:        store,
		repositories: repositories,
		clients:      clients,
	}
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (*Requirement, error) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return nil, ErrInvalidRequirement
	}
	repositoryID := strings.TrimSpace(request.RepositoryID)
	if repositoryID == "" {
		return nil, ErrInvalidRequirement
	}
	priority := strings.TrimSpace(request.Priority)
	if !ValidPriority(priority) {
		return nil, ErrInvalidRequirement
	}
	repo, err := s.repositories.Get(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, registeredrepo.ErrNotFound
		}
		return nil, err
	}
	if repo.Status != registeredrepo.StatusEnabled {
		return nil, ErrRepositoryNotEnabled
	}
	item := &Requirement{
		Title:        title,
		Description:  request.Description,
		Status:       StatusQueued,
		RepositoryID: repositoryID,
		Labels:       EncodeLabels(request.Labels),
		Priority:     priority,
	}
	if err := s.store.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Get(ctx context.Context, requirementID string) (*Requirement, error) {
	item, err := s.store.Get(ctx, requirementID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Requirement, int64, error) {
	return s.store.List(ctx, filter)
}

func (s *Service) Patch(ctx context.Context, requirementID string, request PatchRequest) (*Requirement, error) {
	item, err := s.Get(ctx, requirementID)
	if err != nil {
		return nil, err
	}
	if item.Status != StatusQueued && item.Status != StatusFailed {
		return nil, ErrNotEditable
	}
	if request.Title != nil {
		title := strings.TrimSpace(*request.Title)
		if title == "" {
			return nil, ErrInvalidRequirement
		}
		item.Title = title
	}
	if request.Description != nil {
		item.Description = *request.Description
	}
	if request.Priority != nil {
		priority := strings.TrimSpace(*request.Priority)
		if !ValidPriority(priority) {
			return nil, ErrInvalidRequirement
		}
		item.Priority = priority
	}
	if request.Labels != nil {
		item.Labels = EncodeLabels(*request.Labels)
	}
	if err := s.store.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Publish(ctx context.Context, requirementID string) (*Requirement, error) {
	item, err := s.Get(ctx, requirementID)
	if err != nil {
		return nil, err
	}
	if item.Status != StatusFailed {
		return nil, ErrInvalidTransition
	}
	item.Status = StatusQueued
	item.LastError = ""
	if err := s.store.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Cancel(ctx context.Context, requirementID string) (*Requirement, error) {
	item, err := s.Get(ctx, requirementID)
	if err != nil {
		return nil, err
	}
	if item.Status != StatusQueued && item.Status != StatusFailed {
		return nil, ErrInvalidTransition
	}
	item.Status = StatusCancelled
	if err := s.store.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) Complete(ctx context.Context, requirementID string) (*Requirement, error) {
	item, err := s.Get(ctx, requirementID)
	if err != nil {
		return nil, err
	}
	if item.Status != StatusPublished {
		return nil, ErrInvalidTransition
	}
	item.Status = StatusDone
	now := time.Now()
	item.CompletedAt = &now
	if err := s.store.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}
