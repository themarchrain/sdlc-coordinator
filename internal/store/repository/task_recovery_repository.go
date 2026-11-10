package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	return loadRecoverySnapshot(ctx, r.db, taskID)
}

func loadRecoverySnapshot(ctx context.Context, db *gorm.DB, taskID string) (*RecoverySnapshot, error) {
	var task model.Task
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load recovery task: %w", err)
	}
	snapshot := &RecoverySnapshot{Task: &task, Version: task.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).Order("id ASC").Find(&snapshot.Runs).Error; err != nil {
		return nil, fmt.Errorf("load recovery runs: %w", err)
	}
	runIDs := make([]string, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runIDs = append(runIDs, run.RunID)
	}
	if len(runIDs) != 0 {
		if err := db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.Executions).Error; err != nil {
			return nil, fmt.Errorf("load recovery executions: %w", err)
		}
		if err := db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.ResultSubmissions).Error; err != nil {
			return nil, fmt.Errorf("load recovery submissions: %w", err)
		}
		if err := db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.Writebacks).Error; err != nil {
			return nil, fmt.Errorf("load recovery writebacks: %w", err)
		}
		if err := db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.Effects).Error; err != nil {
			return nil, fmt.Errorf("load recovery effects: %w", err)
		}
		if err := db.WithContext(ctx).Where("run_id IN ?", runIDs).Order("id ASC").Find(&snapshot.DeliveryCheckpoints).Error; err != nil {
			return nil, fmt.Errorf("load recovery delivery checkpoints: %w", err)
		}
	}
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).Order("id ASC").Find(&snapshot.ActionAdmissions).Error; err != nil {
		return nil, fmt.Errorf("load recovery action admissions: %w", err)
	}
	var leases []model.WorkItemLease
	if err := db.WithContext(ctx).Order("scope ASC").Limit(1000).Find(&leases).Error; err != nil {
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
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).First(&audit).Error; err == nil {
		snapshot.MergeGateAudit = &audit
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load recovery merge audit: %w", err)
	}
	var stop model.StopRequest
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).Order("id DESC").First(&stop).Error; err == nil {
		snapshot.Stop = &stop
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load recovery stop: %w", err)
	}
	return snapshot, nil
}

func (r *TaskRecoveryRepository) Accept(ctx context.Context, taskID, actorID string, command RecoveryCommand, proof RecoveryDBProof) (*model.TaskRecovery, bool, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(actorID) == "" || strings.TrimSpace(command.RequestID) == "" || strings.TrimSpace(command.RequestHash) == "" || strings.TrimSpace(command.BlockRevision) == "" || command.CycleMaxAttempts <= 0 {
		return nil, false, ErrInvalidRecoveryCommand
	}
	existing, err := r.GetByRequest(ctx, taskID, command.RequestID)
	if err == nil {
		if recoveryCommandMatches(existing, actorID, command) {
			return existing, true, nil
		}
		return nil, false, ErrRecoveryRequestConflict
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	var accepted *model.TaskRecovery
	err = withLockedTask(ctx, r.db, taskID, func(tx *gorm.DB, task *model.Task, now time.Time) error {
		var current model.TaskRecovery
		lookup := tx.Where("task_id = ? AND request_id = ?", taskID, command.RequestID).First(&current).Error
		if lookup == nil {
			if recoveryCommandMatches(&current, actorID, command) {
				accepted = &current
				return nil
			}
			return ErrRecoveryRequestConflict
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		var stopCount int64
		if err := tx.Model(&model.StopRequest{}).Where("task_id = ?", taskID).Count(&stopCount).Error; err != nil {
			return err
		}
		if stopCount != 0 {
			return ErrRecoveryStateChanged
		}
		if task == nil || task.Status != model.TaskStatusBlocked {
			return ErrRecoveryStateChanged
		}
		if task.RecoveryGeneration != command.ExpectedGeneration {
			return ErrRecoveryRevisionChanged
		}
		currentRunID := ""
		if task.CurrentRunID != nil {
			currentRunID = *task.CurrentRunID
		}
		if proof.TaskVersion != "" && ((proof.TaskID != "" && proof.TaskID != task.TaskID) || proof.TaskVersion != RecoveryTaskVersion(task) || proof.RecoveryGeneration != task.RecoveryGeneration) {
			return ErrRecoveryRevisionChanged
		}
		if proof.CurrentRunID != "" || currentRunID != "" {
			if proof.CurrentRunID != currentRunID {
				return ErrRecoveryRevisionChanged
			}
		}
		if proof.TaskVersion != "" && proof.AttemptCount != task.AttemptCount {
			return ErrRecoveryRevisionChanged
		}
		if proof.SafetyDigest != "" {
			snapshot, snapshotErr := loadRecoverySnapshot(ctx, tx, taskID)
			if snapshotErr != nil {
				return snapshotErr
			}
			if RecoverySafetyDigest(snapshot) != proof.SafetyDigest {
				return ErrRecoveryRevisionChanged
			}
		}
		if command.BlockRevision != "" && command.BlockRevision != RecoveryBlockRevision(task, proof.SafetyDigest) {
			return ErrRecoveryRevisionChanged
		}
		if command.Classification == "fatal" {
			return ErrRecoveryNotEligible
		}
		if command.Classification == "unknown" && (!command.RiskAcknowledged || strings.TrimSpace(command.Reason) == "") {
			return ErrRecoveryRiskRequired
		}
		targetGeneration := task.RecoveryGeneration + 1
		record := &model.TaskRecovery{
			RecoveryID:          model.NewID("recovery"),
			TaskID:              task.TaskID,
			RequestID:           command.RequestID,
			RequestHash:         command.RequestHash,
			SourceGeneration:    task.RecoveryGeneration,
			TargetGeneration:    targetGeneration,
			BlockRevision:       command.BlockRevision,
			SourceRunID:         currentRunID,
			RequestedBy:         actorID,
			RequestedAt:         now,
			Reason:              command.Reason,
			RiskAcknowledged:    command.RiskAcknowledged,
			Classification:      command.Classification,
			FailureSnapshotJSON: command.FailureSnapshotJSON,
			AttemptCountBefore:  task.AttemptCount,
			CycleMaxAttempts:    command.CycleMaxAttempts,
		}
		if err := tx.Create(record).Error; err != nil {
			return fmt.Errorf("create recovery audit: %w", err)
		}
		if err := tx.Model(task).Updates(map[string]any{
			"status":                   model.TaskStatusPending,
			"recovery_generation":      targetGeneration,
			"retry_cycle_base_count":   task.AttemptCount,
			"retry_cycle_max_attempts": command.CycleMaxAttempts,
			"current_run_id":           nil,
			"lease_until":              nil,
			"last_error":               "",
			"updated_at":               now,
		}).Error; err != nil {
			return err
		}
		accepted = record
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return accepted, false, nil
}

func recoveryCommandMatches(existing *model.TaskRecovery, actorID string, command RecoveryCommand) bool {
	if existing == nil {
		return false
	}
	return existing.RequestedBy == actorID &&
		existing.RequestHash == command.RequestHash &&
		existing.BlockRevision == command.BlockRevision &&
		existing.Reason == command.Reason &&
		existing.RiskAcknowledged == command.RiskAcknowledged &&
		existing.SourceGeneration == command.ExpectedGeneration &&
		existing.CycleMaxAttempts == command.CycleMaxAttempts &&
		existing.Classification == command.Classification &&
		existing.FailureSnapshotJSON == command.FailureSnapshotJSON
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
