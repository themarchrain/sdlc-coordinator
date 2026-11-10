package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sdlc-coordinator/internal/model"

	"gorm.io/gorm"
)

type RecoverySnapshot struct {
	Task                *model.Task
	Runs                []model.Run
	Executions          []model.AgentExecution
	ResultSubmissions   []model.ResultSubmission
	Writebacks          []model.Writeback
	Effects             []model.Effect
	ActionAdmissions    []model.ActionAdmission
	WorkItemLeases      []model.WorkItemLease
	DeliveryCheckpoints []model.DeveloperDeliveryCheckpoint
	MergeGateAudit      *model.MergeGateAudit
	Stop                *model.StopRequest
	Version             string
}

type TaskRecoveryRepository struct {
	db *gorm.DB
}

func NewTaskRecoveryRepository(db *gorm.DB) *TaskRecoveryRepository {
	return &TaskRecoveryRepository{db: db}
}

// LoadRecoverySnapshot reads the bounded, read-only evidence needed to assess
// one blocked task. It never changes task state or creates execution records.
func (r *TaskRecoveryRepository) LoadRecoverySnapshot(ctx context.Context, taskID string) (*RecoverySnapshot, error) {
	var task model.Task
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load recovery task: %w", err)
	}
	snapshot := &RecoverySnapshot{Task: &task, Version: task.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")}
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("id ASC").Find(&snapshot.Runs).Error; err != nil {
		return nil, fmt.Errorf("load recovery runs: %w", err)
	}
	runIDs := make([]string, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runIDs = append(runIDs, run.RunID)
	}
	if len(runIDs) != 0 {
		if err := r.db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.Executions).Error; err != nil {
			return nil, fmt.Errorf("load recovery executions: %w", err)
		}
		if err := r.db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.ResultSubmissions).Error; err != nil {
			return nil, fmt.Errorf("load recovery submissions: %w", err)
		}
		if err := r.db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.Writebacks).Error; err != nil {
			return nil, fmt.Errorf("load recovery writebacks: %w", err)
		}
		if err := r.db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.Effects).Error; err != nil {
			return nil, fmt.Errorf("load recovery effects: %w", err)
		}
		if err := r.db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.DeliveryCheckpoints).Error; err != nil {
			return nil, fmt.Errorf("load recovery delivery checkpoints: %w", err)
		}
	}
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("id ASC").Find(&snapshot.ActionAdmissions).Error; err != nil {
		return nil, fmt.Errorf("load recovery action admissions: %w", err)
	}
	var leases []model.WorkItemLease
	if err := r.db.WithContext(ctx).Order("scope ASC").Find(&leases).Error; err != nil {
		return nil, fmt.Errorf("load recovery leases: %w", err)
	}
	for _, lease := range leases {
		if strings.Contains(lease.Scope, taskID) {
			snapshot.WorkItemLeases = append(snapshot.WorkItemLeases, lease)
			continue
		}
		for _, runID := range runIDs {
			if strings.Contains(lease.Scope, runID) {
				snapshot.WorkItemLeases = append(snapshot.WorkItemLeases, lease)
				break
			}
		}
	}
	var audit model.MergeGateAudit
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&audit).Error; err == nil {
		snapshot.MergeGateAudit = &audit
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load recovery merge audit: %w", err)
	}
	var stop model.StopRequest
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("id DESC").First(&stop).Error; err == nil {
		snapshot.Stop = &stop
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load recovery stop: %w", err)
	}
	return snapshot, nil
}

func (r *TaskRecoveryRepository) GetByRequest(ctx context.Context, taskID, requestID string) (*model.TaskRecovery, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(requestID) == "" {
		return nil, ErrNotFound
	}
	var recovery model.TaskRecovery
	err := r.db.WithContext(ctx).Where("task_id = ? AND request_id = ?", taskID, requestID).First(&recovery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get task recovery by request: %w", err)
	}
	return &recovery, nil
}
