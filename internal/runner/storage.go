package runner

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxDurableEventLineBytes = 16 << 20

const quarantineDirName = "quarantine"

func (s *Service) loadDurableExecutions() error {
	root := filepath.Join(s.dataRoot, "executions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("list durable executions: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == quarantineDirName {
			continue
		}
		if !entry.IsDir() || !executionIDPattern.MatchString(name) {
			s.quarantineDurableEntry(root, name, errors.New("unexpected durable entry kind or name"))
			continue
		}
		runtime, err := s.loadDurableExecution(name)
		if err != nil {
			// 单个执行的损坏只影响它自己：隔离取证后继续加载其余，
			// 不允许一个坏目录让整个 runner 拒绝启动。
			s.quarantineDurableEntry(root, name, err)
			continue
		}
		s.runtime[name] = runtime
	}
	return nil
}

func (s *Service) quarantineDurableEntry(root, name string, cause error) {
	quarantineRoot := filepath.Join(root, quarantineDirName)
	target := filepath.Join(quarantineRoot, fmt.Sprintf("%s-%d", name, time.Now().UnixNano()))
	if err := os.MkdirAll(quarantineRoot, 0o755); err != nil {
		log.Printf("runner: create quarantine directory failed, entry %s left in place: %v (cause: %v)", name, err, cause)
		return
	}
	if err := os.Rename(filepath.Join(root, name), target); err != nil {
		log.Printf("runner: quarantine entry %s failed: %v (cause: %v)", name, err, cause)
		return
	}
	log.Printf("runner: quarantined corrupted durable entry %s: %v", name, cause)
}

func (s *Service) loadDurableExecution(directoryID string) (*executionRuntime, error) {
	root := s.executionRoot(directoryID)
	var execution Execution
	if err := readJSONFile(filepath.Join(root, "execution.json"), &execution); err != nil {
		return nil, err
	}
	if err := validateDurableExecution(directoryID, &execution); err != nil {
		return nil, err
	}
	events := readDurableEvents(filepath.Join(root, "events.jsonl"), directoryID)
	result, err := readDurableResult(filepath.Join(root, "structured-result.json"), directoryID)
	if err != nil {
		return nil, err
	}
	// 断号容忍加载后，追加事件必须从已见最大 seq 续接，避免与既有序号冲突。
	nextSeq := int64(1)
	for _, event := range events {
		if event.Seq >= nextSeq {
			nextSeq = event.Seq + 1
		}
	}
	runtime := &executionRuntime{execution: execution, events: events, nextEventSeq: nextSeq, result: result}
	if result != nil {
		if result.ResultHash == "" || result.ResultHash != hashStructuredOutput(result.StructuredOutput) {
			return nil, errors.New("structured-result.json hash is invalid")
		}
		if execution.ResultHash != "" && execution.ResultHash != result.ResultHash {
			return nil, errors.New("structured-result.json hash does not match execution")
		}
	}

	switch execution.Status {
	case StatusSucceeded:
		if result == nil {
			return nil, errors.New("structured-result.json is required for succeeded execution")
		}
	case StatusAccepted, StatusStarting, StatusRunning:
		if result != nil {
			runtime.execution.Status = StatusSucceeded
			runtime.execution.ResultHash = result.ResultHash
			finishedAt := result.CompletedAt
			runtime.execution.FinishedAt = &finishedAt
			if err := s.persistExecutionLocked(runtime); err != nil {
				return nil, err
			}
			if err := s.appendEventLocked(runtime, "succeeded_recovered", nil); err != nil {
				return nil, err
			}
			break
		}
		now := s.now().UTC()
		runtime.execution.Status = StatusInterrupted
		runtime.execution.ErrorCode = "runner_restarted"
		runtime.execution.FinishedAt = &now
		if err := s.persistExecutionLocked(runtime); err != nil {
			return nil, err
		}
		if err := s.appendEventLocked(runtime, "interrupted", map[string]any{"error_code": "runner_restarted"}); err != nil {
			return nil, err
		}
	case StatusFailed, StatusCancelled, StatusInterrupted:
	default:
		return nil, fmt.Errorf("unsupported durable execution status %q", execution.Status)
	}
	return runtime, nil
}

func validateDurableExecution(directoryID string, execution *Execution) error {
	if execution.ExecutionID != directoryID {
		return errors.New("execution.json identity does not match directory")
	}
	for name, value := range map[string]string{
		"task_id": execution.TaskID, "role": execution.Role, "provider": execution.Provider, "request_hash": execution.RequestHash,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("execution.json %s is required", name)
		}
	}
	return nil
}

func readDurableEvents(path, executionID string) []Event {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Printf("runner: read durable events %s: %v", path, err)
		return nil
	}
	defer file.Close()
	events := make([]Event, 0)
	lastSeq := int64(0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxDurableEventLineBytes)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			// 常见形态是进程崩溃留下的尾部半行：保留已解析前缀即可，
			// 追加事件从最大 seq 续接，不因坏行拒载整个执行。
			log.Printf("runner: durable events %s stop at undecodable line after seq %d: %v", executionID, lastSeq, err)
			break
		}
		if event.ExecutionID != executionID {
			log.Printf("runner: durable events %s skip foreign event seq %d", executionID, event.Seq)
			continue
		}
		if event.Seq <= lastSeq {
			log.Printf("runner: durable events %s skip duplicate or out-of-order seq %d", executionID, event.Seq)
			continue
		}
		lastSeq = event.Seq
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("runner: durable events %s scan stopped: %v", executionID, err)
	}
	return events
}

func readDurableResult(path, executionID string) (*Result, error) {
	var result Result
	if err := readJSONFile(path, &result); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if result.ExecutionID != executionID || len(result.StructuredOutput) == 0 || !json.Valid(result.StructuredOutput) || result.CompletedAt.Equal(time.Time{}) {
		return nil, errors.New("structured-result.json identity or content is invalid")
	}
	result.StructuredOutput = compactStructuredOutput(result.StructuredOutput)
	return &result, nil
}

func readJSONFile(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	var raw bytes.Buffer
	encoder := json.NewEncoder(&raw)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return writeBytesAtomic(path, raw.Bytes())
}

func writeBytesAtomic(path string, raw []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish %s: %w", filepath.Base(path), err)
	}
	return nil
}
