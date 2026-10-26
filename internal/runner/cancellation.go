package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrExecutionCancelled = errors.New("execution is durably cancelled")

type CancellationRequest struct {
	StopID string `json:"stop_id"`
	TaskID string `json:"task_id"`
}

type CancellationAck struct {
	StopID                string     `json:"stop_id"`
	ExecutionID           string     `json:"execution_id"`
	TaskID                string     `json:"task_id"`
	CancelIntentPersisted bool       `json:"cancel_intent_persisted"`
	FutureCreateBlocked   bool       `json:"future_create_blocked"`
	ExecutionPresent      bool       `json:"execution_present"`
	ExitConfirmed         bool       `json:"exit_confirmed"`
	Execution             *Execution `json:"execution,omitempty"`
}

type cancellationRecord struct {
	CancellationRequest
	ExecutionID string    `json:"execution_id"`
	RequestedAt time.Time `json:"requested_at"`
}

func (s *Service) CancelDurable(executionID string, request CancellationRequest) (*CancellationAck, error) {
	if !executionIDPattern.MatchString(executionID) || !executionIDPattern.MatchString(request.StopID) || strings.TrimSpace(request.TaskID) == "" || len(request.TaskID) > 128 {
		return nil, errors.New("invalid cancellation identity")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.runtime[executionID]
	if runtime != nil && runtime.execution.TaskID != request.TaskID {
		return nil, ErrConflict
	}
	if previous, ok := s.cancellations[executionID]; ok {
		if previous.TaskID != request.TaskID || previous.StopID != request.StopID {
			return nil, ErrConflict
		}
	} else {
		record := cancellationRecord{CancellationRequest: request, ExecutionID: executionID, RequestedAt: s.now().UTC()}
		if err := writeJSONAtomic(filepath.Join(s.dataRoot, "cancellations", executionID+".json"), record); err != nil {
			return nil, fmt.Errorf("persist cancellation: %w", err)
		}
		s.cancellations[executionID] = record
	}
	ack := &CancellationAck{StopID: request.StopID, TaskID: request.TaskID, ExecutionID: executionID, CancelIntentPersisted: true, FutureCreateBlocked: true, ExecutionPresent: runtime != nil}
	if runtime != nil {
		if runtime.cancel != nil {
			runtime.cancel()
		}
		copy := runtime.execution
		ack.Execution = &copy
		ack.ExitConfirmed = runtime.exited
	}
	return ack, nil
}

func (s *Service) loadCancellations() error {
	root := filepath.Join(s.dataRoot, "cancellations")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	s.cancellations = map[string]cancellationRecord{}
	for _, entry := range entries {
		if !entry.IsDir() && (strings.EqualFold(entry.Name(), "desktop.ini") || strings.EqualFold(entry.Name(), "Thumbs.db") || entry.Name() == ".DS_Store") {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || entry.Name() != id+".json" || !executionIDPattern.MatchString(id) {
			return errors.New("invalid cancellation record path")
		}
		var record cancellationRecord
		if err := readJSONFile(filepath.Join(root, entry.Name()), &record); err != nil {
			return fmt.Errorf("read cancellation %s: %w", id, err)
		}
		if record.ExecutionID != id || !executionIDPattern.MatchString(record.StopID) || strings.TrimSpace(record.TaskID) == "" {
			return fmt.Errorf("invalid cancellation record %s", id)
		}
		s.cancellations[id] = record
	}
	return nil
}
