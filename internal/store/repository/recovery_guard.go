package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

var (
	ErrInvalidRecoveryCommand  = errors.New("invalid recovery command")
	ErrRecoveryRequestConflict = errors.New("recovery request conflicts with existing request")
	ErrRecoveryStateChanged    = errors.New("recovery state changed")
	ErrRecoveryRevisionChanged = errors.New("recovery revision changed")
	ErrRecoveryRiskRequired    = errors.New("recovery risk confirmation required")
	ErrRecoveryNotEligible     = errors.New("recovery is not eligible")
)

type RecoveryCommand struct {
	RequestID           string
	RequestHash         string
	BlockRevision       string
	Reason              string
	RiskAcknowledged    bool
	ExpectedGeneration  uint64
	CycleMaxAttempts    int
	FailureSnapshotJSON string
	Classification      string
}

type RecoveryDBProof struct {
	TaskID             string
	TaskVersion        string
	CurrentRunID       string
	RecoveryGeneration uint64
	AttemptCount       int
	SafetyDigest       string
}

func RecoveryTaskVersion(task *model.Task) string {
	if task == nil {
		return ""
	}
	return task.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

func RecoveryBlockRevision(task *model.Task, safetyDigest string) string {
	if task == nil {
		return ""
	}
	currentRun := ""
	if task.CurrentRunID != nil {
		currentRun = strings.TrimSpace(*task.CurrentRunID)
	}
	raw := strings.Join([]string{
		task.TaskID,
		string(task.Status),
		strconv.Itoa(task.AttemptCount),
		strconv.FormatUint(task.RecoveryGeneration, 10),
		currentRun,
		RecoveryTaskVersion(task),
		strings.TrimSpace(safetyDigest),
	}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// RecoverySafetyDigest hashes only durable safety semantics, excluding
// observation timestamps such as last_observed_at/checked_at.
func RecoverySafetyDigest(snapshot *RecoverySnapshot) string {
	if snapshot == nil {
		return ""
	}
	view := map[string]any{}
	if snapshot.Task != nil {
		currentRun := ""
		if snapshot.Task.CurrentRunID != nil {
			currentRun = *snapshot.Task.CurrentRunID
		}
		view["task"] = []string{
			snapshot.Task.TaskID, string(snapshot.Task.Status), strconv.Itoa(snapshot.Task.AttemptCount),
			strconv.FormatUint(snapshot.Task.RecoveryGeneration, 10), currentRun,
		}
	}
	runs := make([]string, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runs = append(runs, fmt.Sprintf("%s|%s|%d", run.RunID, run.Status, run.RecoveryGeneration))
	}
	view["runs"] = runs
	executions := make([]string, 0, len(snapshot.Executions))
	for _, execution := range snapshot.Executions {
		finished := ""
		if execution.FinishedAt != nil {
			finished = "finished"
		}
		executions = append(executions, strings.Join([]string{
			execution.RunID, execution.ExecutionID, string(execution.Status), execution.RequestHash, finished,
		}, "|"))
	}
	view["executions"] = executions
	submissions := make([]string, 0, len(snapshot.ResultSubmissions))
	for _, submission := range snapshot.ResultSubmissions {
		submissions = append(submissions, fmt.Sprintf("%s|%s|%d", submission.RunID, submission.Status, submission.AttemptCount))
	}
	view["submissions"] = submissions
	writebacks := make([]string, 0, len(snapshot.Writebacks))
	for _, writeback := range snapshot.Writebacks {
		writebacks = append(writebacks, fmt.Sprintf("%s|%s|%s", writeback.WritebackID, writeback.Status, writeback.DedupeKey))
	}
	view["writebacks"] = writebacks
	effects := make([]string, 0, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effects = append(effects, fmt.Sprintf("%s|%s|%t|%s", effect.EffectID, effect.Status, effect.Critical, effect.DedupeKey))
	}
	view["effects"] = effects
	admissions := make([]string, 0, len(snapshot.ActionAdmissions))
	for _, admission := range snapshot.ActionAdmissions {
		admissions = append(admissions, fmt.Sprintf("%s|%s|%s|%d|%t|%t", admission.AdmissionID, admission.Kind, admission.ActionKey, admission.Attempt, admission.NotSentAt != nil, admission.CompletedAt != nil))
	}
	view["admissions"] = admissions
	leases := make([]string, 0, len(snapshot.WorkItemLeases))
	for _, lease := range snapshot.WorkItemLeases {
		leases = append(leases, fmt.Sprintf("%s|%s|%d|%d", lease.Scope, lease.Owner, lease.Generation, lease.LeaseUntil.UnixNano()))
	}
	view["leases"] = leases
	checkpoints := make([]string, 0, len(snapshot.DeliveryCheckpoints))
	for _, checkpoint := range snapshot.DeliveryCheckpoints {
		checkpoints = append(checkpoints, fmt.Sprintf("%s|%s|%s", checkpoint.RunID, checkpoint.BaselineSHA, checkpoint.CommitSHA))
	}
	view["checkpoints"] = checkpoints
	if snapshot.MergeGateAudit != nil {
		view["merge_audit"] = fmt.Sprintf("%s|%s|%s", snapshot.MergeGateAudit.AuditID, snapshot.MergeGateAudit.Decision, snapshot.MergeGateAudit.HeadSHA)
	}
	if snapshot.Stop != nil {
		view["stop"] = fmt.Sprintf("%s|%s|%s", snapshot.Stop.StopID, snapshot.Stop.Scope, snapshot.Stop.DeliveryStatus)
	}
	raw, _ := json.Marshal(view)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
