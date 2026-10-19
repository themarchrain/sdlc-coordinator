package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ManifestFileName         = "workspace-manifest.json"
	ManifestStatusReady      = "ready"
	ReviewModeFull           = "full"
	ReviewModeIncremental    = "incremental"
	FallbackBaseUnavailable  = "base_unavailable"
	FallbackBaseNotAncestor  = "base_not_ancestor"
	DisabledReviewerPushURL  = "disabled://reviewer-read-only"
	DisabledDeveloperPushURL = "disabled://coordinator-managed-delivery"
)

var (
	ErrWorkspaceConflict = errors.New("workspace already exists")
	ErrHeadUnavailable   = errors.New("review head is unavailable")
	safeRunIDPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)
	commitSHAPattern     = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)
)

type GitCredential struct {
	Username string
	Token    string
}

type ReviewerRequest struct {
	WorkspaceRoot string
	// PrivateRunRoot is enabled by the Runner operation service's isolated CLI mode.
	PrivateRunRoot      bool
	TaskID              string
	RunID               string
	Project             string
	RepositoryURL       string
	SourceBranch        string
	TargetBranch        string
	HeadSHA             string
	BaseSHA             string
	RequestedReviewMode string
	Credential          *GitCredential
	// WorkerRole 标记工作区服务的角色(审计进 manifest);空值视为
	// ReviewerWorker 以兼容既有调用方。验证任务复用本请求结构。
	WorkerRole string
}

type RepositoryManifest struct {
	Project             string `json:"project"`
	Path                string `json:"path"`
	RepositoryURL       string `json:"repository_url,omitempty"`
	InitialHeadSHA      string `json:"initial_head_sha,omitempty"`
	DeliveryBranch      string `json:"delivery_branch,omitempty"`
	SourceBranch        string `json:"source_branch"`
	TargetBranch        string `json:"target_branch"`
	RequestedReviewMode string `json:"requested_review_mode"`
	EffectiveReviewMode string `json:"effective_review_mode"`
	FallbackReason      string `json:"fallback_reason,omitempty"`
	BaseSHA             string `json:"base_sha,omitempty"`
	TargetSHA           string `json:"target_sha"`
	HeadSHA             string `json:"head_sha"`
	DiffRange           string `json:"diff_range"`
}

type Manifest struct {
	Version    int                `json:"version"`
	Status     string             `json:"status"`
	TaskID     string             `json:"task_id"`
	RunID      string             `json:"run_id"`
	Role       string             `json:"role"`
	TaskType   string             `json:"task_type,omitempty"`
	Repository RepositoryManifest `json:"repository"`
	PreparedAt time.Time          `json:"prepared_at"`
	// RunnerWorkspacePath 是 Runner 侧 run 工作区路径（远端 prepare 返回值）：
	// 独立文件系统下两侧路径不共享，执行请求必须用该路径；本机形态为空
	// （调用方沿用协调器本地计算值）。
	RunnerWorkspacePath string `json:"runner_workspace_path,omitempty"`
}

func validateReviewerRequest(req ReviewerRequest) (ReviewerRequest, error) {
	req.WorkspaceRoot = strings.TrimSpace(req.WorkspaceRoot)
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.RunID = strings.TrimSpace(req.RunID)
	req.Project = strings.TrimSpace(req.Project)
	req.RepositoryURL = strings.TrimSpace(req.RepositoryURL)
	req.SourceBranch = strings.TrimSpace(req.SourceBranch)
	req.TargetBranch = strings.TrimSpace(req.TargetBranch)
	req.HeadSHA = strings.ToLower(strings.TrimSpace(req.HeadSHA))
	req.BaseSHA = strings.ToLower(strings.TrimSpace(req.BaseSHA))
	req.RequestedReviewMode = strings.ToLower(strings.TrimSpace(req.RequestedReviewMode))

	for name, value := range map[string]string{
		"workspace root": req.WorkspaceRoot,
		"task id":        req.TaskID,
		"run id":         req.RunID,
		"project":        req.Project,
		"repository url": req.RepositoryURL,
		"source branch":  req.SourceBranch,
		"target branch":  req.TargetBranch,
		"head sha":       req.HeadSHA,
	} {
		if value == "" {
			return ReviewerRequest{}, fmt.Errorf("%s is required", name)
		}
	}
	if !safeRunIDPattern.MatchString(req.RunID) {
		return ReviewerRequest{}, fmt.Errorf("run id %q is not a safe path component", req.RunID)
	}
	if !commitSHAPattern.MatchString(req.HeadSHA) {
		return ReviewerRequest{}, fmt.Errorf("head sha must be a 40-64 character hexadecimal commit id")
	}
	if req.BaseSHA != "" && !commitSHAPattern.MatchString(req.BaseSHA) {
		return ReviewerRequest{}, fmt.Errorf("base sha must be a 40-64 character hexadecimal commit id")
	}
	if req.RequestedReviewMode != ReviewModeFull && req.RequestedReviewMode != ReviewModeIncremental {
		return ReviewerRequest{}, fmt.Errorf("unsupported review mode %q", req.RequestedReviewMode)
	}
	if strings.Contains(req.RepositoryURL, "://") {
		parsed, err := url.Parse(req.RepositoryURL)
		if err != nil {
			return ReviewerRequest{}, fmt.Errorf("parse repository url: %w", err)
		}
		if parsed.User != nil {
			return ReviewerRequest{}, errors.New("repository url must not contain credentials")
		}
	}
	return req, nil
}

// effectiveWorkerRole 归一请求角色:空值视为 ReviewerWorker 以兼容既有
// 调用方;验证任务显式带 TesterWorker。写入 manifest 与复用匹配共用本
// 归一,保证两个口径永不漂移。
func effectiveWorkerRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "ReviewerWorker"
	}
	return role
}

func manifestMatchesReviewerRequest(manifest *Manifest, req ReviewerRequest) bool {
	return manifest != nil &&
		manifest.Status == ManifestStatusReady &&
		manifest.TaskID == req.TaskID &&
		manifest.RunID == req.RunID &&
		manifest.Role == effectiveWorkerRole(req.WorkerRole) &&
		manifest.Repository.Project == req.Project &&
		manifest.Repository.SourceBranch == req.SourceBranch &&
		manifest.Repository.TargetBranch == req.TargetBranch &&
		strings.EqualFold(manifest.Repository.HeadSHA, req.HeadSHA)
}

func readManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("decode workspace manifest: %w", err)
	}
	return &manifest, nil
}

func writeManifestAtomic(path string, manifest *Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace manifest: %w", err)
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workspace-manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary workspace manifest: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary workspace manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary workspace manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary workspace manifest: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish workspace manifest: %w", err)
	}
	return nil
}
