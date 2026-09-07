package admin

import "time"

const (
	defaultPageSize = 20
	maximumPageSize = 100
)

type PageRequest struct {
	Page     int
	PageSize int
}

func (request PageRequest) Normalize() PageRequest {
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

type TaskFilter struct {
	PageRequest
	Status       string
	Role         string
	Type         string
	Repository   string
	RepositoryID string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

type RunFilter struct {
	PageRequest
	Status       string
	WorkerRole   string
	TaskID       string
	RepositoryID string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

type EntityFilter struct {
	PageRequest
	TaskID       string
	RunID        string
	Status       string
	Kind         string
	Source       string
	Type         string
	Role         string
	RepositoryID string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

type DependencyFilter struct {
	PageRequest
	ParentTaskID string
	ChildTaskID  string
	Relation     string
	Status       string
	RepositoryID string
	CreatedFrom  *time.Time
	CreatedTo    *time.Time
}

type OverviewFilter struct {
	RepositoryID string
}

type Overview struct {
	TasksByStatus      map[string]int64
	RunsByStatus       map[string]int64
	WritebacksByStatus map[string]int64
	EffectsByStatus    map[string]int64
	// SubmissionsByNormalization is the thin-normalizer accounting for the
	// MCP result channel: clean (byte identical), normalized (repairs
	// applied) and rejected (validation attempts that never became a
	// submission).
	SubmissionsByNormalization map[string]int64
	RecentFailures             []FailureSummary
	GeneratedAt                time.Time
}

type FailureSummary struct {
	EntityType string
	EntityID   string
	TaskID     string
	RunID      string
	Status     string
	Error      string
	OccurredAt time.Time
}

type TimelineEvent struct {
	EventID    string
	OccurredAt time.Time
	Category   string
	Action     string
	Status     string
	Title      string
	Summary    string
	EntityType string
	EntityID   string
}

type MetricsWindow string

const (
	MetricsWindow7Days  MetricsWindow = "7d"
	MetricsWindow30Days MetricsWindow = "30d"
	MetricsWindowAll    MetricsWindow = "all"
)

type MetricsFilter struct {
	Window       MetricsWindow
	Type         string
	RepositoryID string
	Now          time.Time
}

type RatioMetric struct {
	Numerator   int64
	Denominator int64
	Value       *float64
}

type RunDurationMetric struct {
	WorkerRole     string
	SampleCount    int
	AverageSeconds float64
	P50Seconds     float64
	P90Seconds     float64
}

type AutomationMetrics struct {
	Window                 MetricsWindow
	Type                   string
	TaskSampleCount        int64
	CurrentSuccess         RatioMetric
	FirstRunSuccess        RatioMetric
	InterventionTriggered  RatioMetric
	RecoveredAfterBlock    RatioMetric
	SuccessfulRunDurations []RunDurationMetric
	DailySeries            []DailyRatioPoint
	GeneratedAt            time.Time
}

// DailyRatioPoint 是 metrics 按日切分的比率序列：日期 + 4 个比率值（该日无样本时为 nil）。
type DailyRatioPoint struct {
	Date                  string
	CurrentSuccess        *float64
	FirstRunSuccess       *float64
	InterventionTriggered *float64
	RecoveredAfterBlock   *float64
}

// DailyCount 是 business_summary 按日计数的数量序列。
type DailyCount struct {
	Date  string
	Count int64
}

// DailyAverageSeconds 是成功 Run 平均耗时的按日序列（口径与窗口均值一致）。
type DailyAverageSeconds struct {
	Date        string
	Seconds     float64
	SampleCount int64
}
