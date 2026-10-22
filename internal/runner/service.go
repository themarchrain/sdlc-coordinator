package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

var executionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type Service struct {
	mu            sync.Mutex
	dataRoot      string
	providers     map[string]Provider
	runtime       map[string]*executionRuntime
	cancellations map[string]cancellationRecord
	now           func() time.Time
	logger        *zap.Logger
}

// Option 在构造期调整 Service 装配,只在 NewService 内应用一次。
type Option func(*Service)

// WithLogger 注入服务实例的应用日志器,覆盖构造期恢复与异步执行期的诊断。
// 未注入或注入 nil 时回退 zap.NewNop():回退只服务测试与最小装配,生产入口
// cmd/agent-runner 必须显式传入已创建的 runner logger。Service 只使用注入的
// logger,不关闭也不替换它。
func WithLogger(logger *zap.Logger) Option {
	return func(service *Service) {
		service.logger = logger
	}
}

type executionRuntime struct {
	execution    Execution
	request      CreateExecutionRequest
	events       []Event
	nextEventSeq int64
	result       *Result
	cancel       context.CancelFunc
	exited       bool
}

func NewService(dataRoot string, providers map[string]Provider, options ...Option) (*Service, error) {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return nil, errors.New("runner data root is required")
	}
	executionsRoot := filepath.Join(dataRoot, "executions")
	if err := os.MkdirAll(executionsRoot, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(executionsRoot, 0o700); err != nil {
		return nil, err
	}
	registry := make(map[string]Provider, len(providers))
	for name, provider := range providers {
		if strings.TrimSpace(name) != "" && provider != nil {
			registry[name] = provider
		}
	}
	service := &Service{dataRoot: dataRoot, providers: registry, runtime: map[string]*executionRuntime{}, now: time.Now}
	// options 必须在 loadDurableExecutions 前应用:构造期恢复(隔离、坏行、
	// 序号异常)的诊断要进入注入的 logger,而不是丢进回退 Nop。
	for _, option := range options {
		option(service)
	}
	if service.logger == nil {
		// 回退只存在于测试/最小装配:生产入口一律显式注入 logger。
		// Nop 意味着漏传时丢的是日志不是行为。
		service.logger = zap.NewNop()
	}
	if err := service.loadCancellations(); err != nil {
		return nil, err
	}
	if err := service.loadDurableExecutions(); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Create(_ context.Context, request CreateExecutionRequest) (*Execution, error) {
	normalized, hash, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if stop, ok := s.cancellations[normalized.ExecutionID]; ok {
		s.mu.Unlock()
		if stop.TaskID != normalized.TaskID {
			return nil, ErrConflict
		}
		return nil, ErrExecutionCancelled
	}
	if existing, ok := s.runtime[normalized.ExecutionID]; ok {
		if existing.execution.RequestHash != hash {
			s.mu.Unlock()
			return nil, ErrConflict
		}
		copy := existing.execution
		s.mu.Unlock()
		return &copy, nil
	}
	provider := s.providers[normalized.Provider.Name]
	if provider == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("provider %q is not registered", normalized.Provider.Name)
	}
	createdAt := s.now().UTC()
	runtime := &executionRuntime{execution: Execution{
		ExecutionID: normalized.ExecutionID, TaskID: normalized.TaskID, Role: normalized.Role,
		Provider: normalized.Provider.Name, Status: StatusAccepted, RequestHash: hash, CreatedAt: createdAt,
	}, request: normalized, nextEventSeq: 1}
	s.runtime[normalized.ExecutionID] = runtime
	if err := os.MkdirAll(s.executionRoot(normalized.ExecutionID), 0o755); err != nil {
		delete(s.runtime, normalized.ExecutionID)
		s.mu.Unlock()
		return nil, err
	}
	if err := s.persistExecutionLocked(runtime); err != nil {
		delete(s.runtime, normalized.ExecutionID)
		s.mu.Unlock()
		return nil, err
	}
	if err := s.appendEventLocked(runtime, "accepted", nil); err != nil {
		_ = os.RemoveAll(s.executionRoot(normalized.ExecutionID))
		delete(s.runtime, normalized.ExecutionID)
		s.mu.Unlock()
		return nil, fmt.Errorf("persist accepted event for %s: %w", normalized.ExecutionID, err)
	}
	execution := runtime.execution
	ctx, cancel := context.WithCancel(context.Background())
	runtime.cancel = cancel
	s.mu.Unlock()

	go s.run(ctx, provider, runtime)
	return &execution, nil
}

func (s *Service) run(ctx context.Context, provider Provider, runtime *executionRuntime) {
	s.mu.Lock()
	_, stopped := s.cancellations[runtime.execution.ExecutionID]
	s.mu.Unlock()
	if stopped {
		s.finishFailure(runtime, StatusCancelled, "cancelled", "execution cancelled")
		return
	}
	executionCtx := ctx
	cancel := func() {}
	if runtime.request.TimeoutSeconds > 0 {
		executionCtx, cancel = context.WithTimeout(ctx, time.Duration(runtime.request.TimeoutSeconds)*time.Second)
	}
	defer cancel()

	s.transition(runtime, StatusStarting, "starting", true)
	s.transition(runtime, StatusRunning, "running", false)
	emit := func(eventType string, payload map[string]any) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.appendEventLocked(runtime, eventType, payload); err != nil {
			s.logger.Error("append provider event failed",
				zap.String("execution_id", runtime.execution.ExecutionID),
				zap.String("event_type", eventType),
				zap.Error(err))
		}
	}
	request := runtime.request
	raw, err := provider.Execute(executionCtx, ProviderRequest{
		ExecutionID: request.ExecutionID, TaskID: request.TaskID, Role: request.Role,
		WorkspacePath: request.WorkspacePath, PromptFile: request.PromptFile, SchemaFile: request.SchemaFile,
		Provider: request.Provider, ResultSubmission: request.ResultSubmission,
	}, emit)
	if err != nil {
		if errors.Is(executionCtx.Err(), context.DeadlineExceeded) {
			s.finishFailure(runtime, StatusFailed, "execution_timeout", "execution timed out")
		} else if errors.Is(executionCtx.Err(), context.Canceled) {
			s.finishFailure(runtime, StatusCancelled, "cancelled", "execution cancelled")
		} else {
			s.finishFailure(runtime, StatusFailed, "provider_failed", redactFailureMessage(err.Error(), request.Provider, request.ResultSubmission))
		}
		return
	}
	if len(raw) == 0 || !json.Valid(raw) {
		s.finishFailure(runtime, StatusFailed, "invalid_structured_result", "provider returned invalid structured result")
		return
	}
	completedAt := s.now().UTC()
	structuredOutput := compactStructuredOutput(raw)
	resultHash := hashBytes(structuredOutput)
	result := &Result{ExecutionID: request.ExecutionID, ResultHash: resultHash, StructuredOutput: structuredOutput, CompletedAt: completedAt}
	s.mu.Lock()
	if _, stopped := s.cancellations[request.ExecutionID]; stopped {
		s.mu.Unlock()
		s.finishFailure(runtime, StatusCancelled, "cancelled", "execution cancelled")
		return
	}
	if err := writeJSONAtomic(filepath.Join(s.executionRoot(request.ExecutionID), "structured-result.json"), result); err != nil {
		s.mu.Unlock()
		s.finishFailure(runtime, StatusFailed, "result_persistence_failed", err.Error())
		return
	}
	runtime.result = result
	runtime.exited = true
	runtime.execution.ResultHash = resultHash
	runtime.execution.Status = StatusSucceeded
	runtime.execution.FinishedAt = &completedAt
	s.persistAndEmitLocked(runtime, "succeeded", nil)
	s.mu.Unlock()
}

func (s *Service) transition(runtime *executionRuntime, status Status, eventType string, started bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime.execution.Status = status
	if started && runtime.execution.StartedAt == nil {
		now := s.now().UTC()
		runtime.execution.StartedAt = &now
	}
	s.persistAndEmitLocked(runtime, eventType, nil)
}

func (s *Service) finishFailure(runtime *executionRuntime, status Status, code, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	if _, stopped := s.cancellations[runtime.execution.ExecutionID]; stopped {
		status, code, message = StatusCancelled, "cancelled", "execution cancelled"
	}
	runtime.exited = true
	runtime.execution.Status = status
	runtime.execution.ErrorCode = code
	runtime.execution.ErrorMessage = strings.TrimSpace(message)
	runtime.execution.FinishedAt = &now
	s.persistAndEmitLocked(runtime, string(status), map[string]any{"error_code": code})
}

// persistAndEmitLocked 持久化 execution 快照并追加生命周期事件；两者都发生在
// 异步执行路径上，失败不再阻断执行，但必须记录日志以便诊断事件流断裂。
func (s *Service) persistAndEmitLocked(runtime *executionRuntime, eventType string, payload map[string]any) {
	if err := s.persistExecutionLocked(runtime); err != nil {
		s.logger.Error("persist execution snapshot failed",
			zap.String("execution_id", runtime.execution.ExecutionID),
			zap.Error(err))
	}
	if err := s.appendEventLocked(runtime, eventType, payload); err != nil {
		s.logger.Error("append execution event failed",
			zap.String("execution_id", runtime.execution.ExecutionID),
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}

func redactFailureMessage(message string, provider ProviderConfig, channel *ResultSubmissionChannel) string {
	return RedactSecrets(message, ProviderSecrets(provider, channel))
}

func (s *Service) Get(executionID string) (*Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.runtime[executionID]
	if runtime == nil {
		return nil, ErrNotFound
	}
	copy := runtime.execution
	return &copy, nil
}

func (s *Service) Events(executionID string, after int64) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.runtime[executionID]
	if runtime == nil {
		return nil, ErrNotFound
	}
	result := make([]Event, 0, len(runtime.events))
	for _, event := range runtime.events {
		if event.Seq > after {
			result = append(result, event)
		}
	}
	return result, nil
}

func (s *Service) Result(executionID string) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.runtime[executionID]
	if runtime == nil {
		return nil, ErrNotFound
	}
	if runtime.result == nil {
		return nil, ErrNotReady
	}
	copy := *runtime.result
	copy.StructuredOutput = append(json.RawMessage(nil), runtime.result.StructuredOutput...)
	return &copy, nil
}

func (s *Service) Cancel(executionID string) (*Execution, error) {
	s.mu.Lock()
	runtime := s.runtime[executionID]
	if runtime == nil {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	if runtime.cancel != nil {
		runtime.cancel()
	}
	copy := runtime.execution
	s.mu.Unlock()
	return &copy, nil
}

// Shutdown 取消全部未终态的执行并等待它们收敛到终态，供进程优雅退出使用：
// provider 层的取消会终止 CLI 子进程树，避免 runner 收到 SIGTERM 后留下孤儿
// 进程继续消耗模型 API 与占用 workspace。ctx 到期时立即返回，未收敛的执行由
// 重启后的 loadDurableExecutions 标记 interrupted 兜底。
func (s *Service) Shutdown(ctx context.Context) {
	s.mu.Lock()
	for _, runtime := range s.runtime {
		if runtime.cancel != nil {
			runtime.cancel()
		}
	}
	for {
		active := 0
		for _, runtime := range s.runtime {
			switch runtime.execution.Status {
			case StatusSucceeded, StatusFailed, StatusCancelled, StatusInterrupted:
			default:
				active++
			}
		}
		s.mu.Unlock()
		if active == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Millisecond):
		}
		s.mu.Lock()
	}
}

func (s *Service) Delete(executionID string) error {
	executionID = strings.TrimSpace(executionID)
	if !executionIDPattern.MatchString(executionID) {
		return errors.New("execution_id is not a safe path component")
	}
	s.mu.Lock()
	runtime := s.runtime[executionID]
	if runtime != nil {
		if runtime.execution.Status != StatusSucceeded {
			s.mu.Unlock()
			return ErrNotPurgeable
		}
		delete(s.runtime, executionID)
		root := s.executionRoot(executionID)
		s.mu.Unlock()
		return os.RemoveAll(root)
	}
	root := s.executionRoot(executionID)
	s.mu.Unlock()
	var execution Execution
	if err := readJSONFile(filepath.Join(root, "execution.json"), &execution); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Stat(root); errors.Is(statErr, os.ErrNotExist) {
				return nil
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return ErrNotPurgeable
	}
	if execution.Status != StatusSucceeded {
		return ErrNotPurgeable
	}
	return os.RemoveAll(root)
}

func (s *Service) appendEventLocked(runtime *executionRuntime, eventType string, payload map[string]any) error {
	event := Event{ExecutionID: runtime.execution.ExecutionID, Seq: runtime.nextEventSeq, Type: eventType, At: s.now().UTC(), Payload: payload}
	runtime.nextEventSeq++
	runtime.events = append(runtime.events, event)
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	path := filepath.Join(s.executionRoot(runtime.execution.ExecutionID), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (s *Service) persistExecutionLocked(runtime *executionRuntime) error {
	return writeJSONAtomic(filepath.Join(s.executionRoot(runtime.execution.ExecutionID), "execution.json"), &runtime.execution)
}

func (s *Service) executionRoot(executionID string) string {
	return filepath.Join(s.dataRoot, "executions", executionID)
}

func normalizeRequest(request CreateExecutionRequest) (CreateExecutionRequest, string, error) {
	request.ExecutionID = strings.TrimSpace(request.ExecutionID)
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.Role = strings.TrimSpace(request.Role)
	request.WorkspacePath = strings.TrimSpace(request.WorkspacePath)
	request.PromptFile = strings.TrimSpace(request.PromptFile)
	request.SchemaFile = strings.TrimSpace(request.SchemaFile)
	request.Provider.Name = strings.TrimSpace(request.Provider.Name)
	for name, value := range map[string]string{
		"execution_id": request.ExecutionID, "task_id": request.TaskID, "role": request.Role,
		"workspace_path": request.WorkspacePath, "prompt_file": request.PromptFile,
		"schema_file": request.SchemaFile, "provider.name": request.Provider.Name,
	} {
		if value == "" {
			return CreateExecutionRequest{}, "", fmt.Errorf("%s is required", name)
		}
	}
	if !executionIDPattern.MatchString(request.ExecutionID) {
		return CreateExecutionRequest{}, "", errors.New("execution_id is not a safe path component")
	}
	if request.TimeoutSeconds < 0 {
		return CreateExecutionRequest{}, "", errors.New("timeout_seconds must not be negative")
	}
	if request.ResultSubmission != nil {
		request.ResultSubmission.EndpointURL = strings.TrimSpace(request.ResultSubmission.EndpointURL)
		request.ResultSubmission.BearerToken = strings.TrimSpace(request.ResultSubmission.BearerToken)
		if request.ResultSubmission.EndpointURL == "" {
			return CreateExecutionRequest{}, "", errors.New("result_submission.endpoint_url is required")
		}
	}
	headerNames := make([]string, 0, len(request.Provider.Headers))
	for name := range request.Provider.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	hashInput := struct {
		ExecutionID, TaskID, Role, WorkspacePath, PromptFile, SchemaFile string
		ProviderName, BaseURL, Model, Effort                             string
		Revision, TimeoutSeconds                                         int64
		HeaderNames                                                      []string
	}{
		request.ExecutionID, request.TaskID, request.Role, request.WorkspacePath, request.PromptFile, request.SchemaFile,
		request.Provider.Name, request.Provider.BaseURL, request.Provider.Model, request.Provider.Effort,
		request.Provider.Revision, request.TimeoutSeconds, headerNames,
	}
	raw, _ := json.Marshal(hashInput)
	return request, hashBytes(raw), nil
}

func RequestHash(request CreateExecutionRequest) (string, error) {
	_, hash, err := normalizeRequest(request)
	return hash, err
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func hashStructuredOutput(raw []byte) string {
	return hashBytes(compactStructuredOutput(raw))
}

func compactStructuredOutput(raw []byte) json.RawMessage {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		return append(json.RawMessage(nil), compact.Bytes()...)
	}
	return append(json.RawMessage(nil), raw...)
}
