package events

type EventKind string

const (
	EventKindChangeRequestUpdated         EventKind = "change_request_updated"
	EventKindWorkItemUpdated              EventKind = "work_item_updated"
	EventKindDeveloperResumeRequested     EventKind = "developer_resume_requested"
	EventKindMergeRequestReworkRequested  EventKind = "merge_request_rework_requested"
	EventKindMergeRequestLifecycleChanged EventKind = "merge_request_lifecycle_changed"
)

type NormalizedEvent struct {
	Source        string         `json:"source"`
	Kind          EventKind      `json:"kind"`
	EventType     string         `json:"event_type"`
	EventAction   string         `json:"event_action"`
	ProjectID     string         `json:"project_id"`
	ProjectPath   string         `json:"project_path"`
	RepositoryURL string         `json:"repository_url"`
	ObjectIID     int            `json:"object_iid"`
	ObjectURL     string         `json:"object_url"`
	Title         string         `json:"title"`
	Description   string         `json:"description,omitempty"`
	SourceBranch  string         `json:"source_branch,omitempty"`
	TargetBranch  string         `json:"target_branch,omitempty"`
	HeadSHA       string         `json:"head_sha,omitempty"`
	BaseSHA       string         `json:"base_sha,omitempty"`
	Version       string         `json:"version"`
	DedupeKey     string         `json:"dedupe_key"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type NormalizationResult struct {
	Event        *NormalizedEvent
	Ignored      bool
	IgnoreReason string
}

func ignoredNormalization(reason string) (*NormalizationResult, error) {
	return &NormalizationResult{Ignored: true, IgnoreReason: reason}, nil
}
