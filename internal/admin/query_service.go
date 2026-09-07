package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
	"sdlc-coordinator/internal/registeredrepo"
	"sdlc-coordinator/internal/store/repository"

	"gorm.io/gorm"
)

type QueryService struct {
	db *gorm.DB
}

func NewQueryService(db *gorm.DB) *QueryService {
	return &QueryService{db: db}
}

func (service *QueryService) ListTasks(ctx context.Context, filter TaskFilter) (Page[model.Task], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.Task]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.Task{})

	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("status = ?", value)
	}
	if value := strings.TrimSpace(filter.Role); value != "" {
		query = query.Where("role = ?", value)
	}
	if value := strings.TrimSpace(filter.Type); value != "" {
		query = query.Where("type = ?", value)
	}
	if value := strings.TrimSpace(filter.Repository); value != "" {
		query = query.Where("(source_repo = ? OR target_repo = ?)", value, value)
	}
	query = applyDirectRepositoryFilter(query, filter.RepositoryID)
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at <= ?", *filter.CreatedTo)
	}

	return listPage[model.Task](query, filter.PageRequest, "tasks")
}

func (service *QueryService) GetTask(ctx context.Context, taskID string) (*model.Task, error) {
	var task model.Task
	if err := service.db.WithContext(ctx).Where("task_id = ?", strings.TrimSpace(taskID)).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get admin task: %w", err)
	}
	return &task, nil
}

func (service *QueryService) ListRuns(ctx context.Context, filter RunFilter) (Page[model.Run], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.Run]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.Run{})

	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("status = ?", value)
	}
	if value := strings.TrimSpace(filter.WorkerRole); value != "" {
		query = query.Where("worker_role = ?", value)
	}
	if value := strings.TrimSpace(filter.TaskID); value != "" {
		query = query.Where("task_id = ?", value)
	}
	query = applyJoinedTaskRepositoryFilter(query, filter.RepositoryID)
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at <= ?", *filter.CreatedTo)
	}

	return listPage[model.Run](query, filter.PageRequest, "runs")
}

func (service *QueryService) GetRun(ctx context.Context, runID string) (*model.Run, error) {
	var run model.Run
	if err := service.db.WithContext(ctx).Where("run_id = ?", strings.TrimSpace(runID)).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get admin run: %w", err)
	}
	return &run, nil
}

func (service *QueryService) ListArtifacts(ctx context.Context, filter EntityFilter) (Page[model.Artifact], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.Artifact]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.Artifact{})
	query = applyTaskRunKindFilter(query, filter)
	query = applyJoinedTaskRepositoryFilter(query, filter.RepositoryID)
	query = applyCreatedFilter(query, filter.CreatedFrom, filter.CreatedTo)
	return listPage[model.Artifact](query, filter.PageRequest, "artifacts")
}

func (service *QueryService) GetArtifact(ctx context.Context, artifactID string) (*model.Artifact, error) {
	return getByBusinessID[model.Artifact](service.db.WithContext(ctx), "artifact_id", artifactID, "artifact")
}

func (service *QueryService) ListDependencies(ctx context.Context, filter DependencyFilter) (Page[model.Dependency], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.Dependency]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.Dependency{})
	if value := strings.TrimSpace(filter.ParentTaskID); value != "" {
		query = query.Where("parent_task_id = ?", value)
	}
	if value := strings.TrimSpace(filter.ChildTaskID); value != "" {
		query = query.Where("child_task_id = ?", value)
	}
	if value := strings.TrimSpace(filter.Relation); value != "" {
		query = query.Where("relation = ?", value)
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		query = query.Where("status = ?", value)
	}
	query = applyDependencyRepositoryFilter(query, filter.RepositoryID)
	query = applyCreatedFilter(query, filter.CreatedFrom, filter.CreatedTo)
	page, err := listPage[model.Dependency](query, filter.PageRequest, "dependencies")
	if err != nil {
		return Page[model.Dependency]{}, err
	}
	if err := service.attachDependencyRepositories(ctx, page.Items); err != nil {
		return Page[model.Dependency]{}, err
	}
	return page, nil
}

func (service *QueryService) GetDependency(ctx context.Context, dependencyID string) (*model.Dependency, error) {
	return getByBusinessID[model.Dependency](service.db.WithContext(ctx), "dependency_id", dependencyID, "dependency")
}

func (service *QueryService) ListWritebacks(ctx context.Context, filter EntityFilter) (Page[model.Writeback], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.Writeback]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.Writeback{})
	query = applyTaskRunKindFilter(query, filter)
	query = applyJoinedTaskRepositoryFilter(query, filter.RepositoryID)
	query = applyStatusFilter(query, filter.Status)
	query = applyCreatedFilter(query, filter.CreatedFrom, filter.CreatedTo)
	return listPage[model.Writeback](query, filter.PageRequest, "writebacks")
}

func (service *QueryService) GetWriteback(ctx context.Context, writebackID string) (*model.Writeback, error) {
	return getByBusinessID[model.Writeback](service.db.WithContext(ctx), "writeback_id", writebackID, "writeback")
}

func (service *QueryService) ListEffects(ctx context.Context, filter EntityFilter) (Page[model.Effect], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.Effect]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.Effect{})
	query = applyTaskRunKindFilter(query, filter)
	query = applyJoinedTaskRepositoryFilter(query, filter.RepositoryID)
	query = applyStatusFilter(query, filter.Status)
	query = applyCreatedFilter(query, filter.CreatedFrom, filter.CreatedTo)
	return listPage[model.Effect](query, filter.PageRequest, "effects")
}

func (service *QueryService) GetEffect(ctx context.Context, effectID string) (*model.Effect, error) {
	return getByBusinessID[model.Effect](service.db.WithContext(ctx), "effect_id", effectID, "effect")
}

func (service *QueryService) ListInboundEvents(ctx context.Context, filter EntityFilter) (Page[model.InboundEvent], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.InboundEvent]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.InboundEvent{})
	query = applyStatusFilter(query, filter.Status)
	if value := strings.TrimSpace(filter.Source); value != "" {
		query = query.Where("source = ?", value)
	}
	if value := strings.TrimSpace(filter.Type); value != "" {
		query = query.Where("event_type = ?", value)
	}
	query = applyDirectRepositoryFilter(query, filter.RepositoryID)
	query = applyCreatedFilter(query, filter.CreatedFrom, filter.CreatedTo)
	return listPage[model.InboundEvent](query, filter.PageRequest, "inbound events")
}

func (service *QueryService) GetInboundEvent(ctx context.Context, eventID string) (*model.InboundEvent, error) {
	return getByBusinessID[model.InboundEvent](service.db.WithContext(ctx), "event_id", eventID, "inbound event")
}

func (service *QueryService) ListResultSubmissions(ctx context.Context, filter EntityFilter) (Page[model.ResultSubmission], error) {
	if err := service.ensureRepository(ctx, filter.RepositoryID); err != nil {
		return Page[model.ResultSubmission]{}, err
	}
	query := service.db.WithContext(ctx).Model(&model.ResultSubmission{})
	if value := strings.TrimSpace(filter.TaskID); value != "" {
		query = query.Where("task_id = ?", value)
	}
	if value := strings.TrimSpace(filter.RunID); value != "" {
		query = query.Where("run_id = ?", value)
	}
	query = applyStatusFilter(query, filter.Status)
	if value := strings.TrimSpace(filter.Source); value != "" {
		query = query.Where("source = ?", value)
	}
	if value := strings.TrimSpace(filter.Role); value != "" {
		query = query.Where("role = ?", value)
	}
	query = applyJoinedTaskRepositoryFilter(query, filter.RepositoryID)
	query = applyCreatedFilter(query, filter.CreatedFrom, filter.CreatedTo)
	return listPage[model.ResultSubmission](query, filter.PageRequest, "result submissions")
}

func (service *QueryService) GetResultSubmission(ctx context.Context, submissionID string) (*model.ResultSubmission, error) {
	return getByBusinessID[model.ResultSubmission](service.db.WithContext(ctx), "submission_id", submissionID, "result submission")
}

func (service *QueryService) ensureRepository(ctx context.Context, repositoryID string) error {
	id := strings.TrimSpace(repositoryID)
	if id == "" {
		return nil
	}
	var total int64
	if err := service.db.WithContext(ctx).Table("registered_repositories").Where("repository_id = ?", id).Count(&total).Error; err != nil {
		return fmt.Errorf("lookup repository: %w", err)
	}
	if total == 0 {
		return registeredrepo.ErrNotFound
	}
	return nil
}

func applyDirectRepositoryFilter(query *gorm.DB, repositoryID string) *gorm.DB {
	if value := strings.TrimSpace(repositoryID); value != "" {
		return query.Where("repository_id = ?", value)
	}
	return query
}

func applyJoinedTaskRepositoryFilter(query *gorm.DB, repositoryID string) *gorm.DB {
	if value := strings.TrimSpace(repositoryID); value != "" {
		return query.Where("task_id IN (SELECT task_id FROM tasks WHERE repository_id = ?)", value)
	}
	return query
}

func applyDependencyRepositoryFilter(query *gorm.DB, repositoryID string) *gorm.DB {
	if value := strings.TrimSpace(repositoryID); value != "" {
		return query.Where(
			"parent_task_id IN (SELECT task_id FROM tasks WHERE repository_id = ?) OR child_task_id IN (SELECT task_id FROM tasks WHERE repository_id = ?)",
			value, value,
		)
	}
	return query
}

func (service *QueryService) attachDependencyRepositories(ctx context.Context, items []model.Dependency) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items)*2)
	seen := make(map[string]struct{}, len(items)*2)
	for _, item := range items {
		for _, id := range []string{item.ParentTaskID, item.ChildTaskID} {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	var tasks []model.Task
	if err := service.db.WithContext(ctx).Select("task_id", "repository_id").Where("task_id IN ?", ids).Find(&tasks).Error; err != nil {
		return fmt.Errorf("load dependency repositories: %w", err)
	}
	byID := make(map[string]*string, len(tasks))
	for i := range tasks {
		byID[tasks[i].TaskID] = tasks[i].RepositoryID
	}
	for i := range items {
		items[i].ParentRepositoryID = byID[items[i].ParentTaskID]
		items[i].ChildRepositoryID = byID[items[i].ChildTaskID]
	}
	return nil
}

func applyTaskRunKindFilter(query *gorm.DB, filter EntityFilter) *gorm.DB {
	if value := strings.TrimSpace(filter.TaskID); value != "" {
		query = query.Where("task_id = ?", value)
	}
	if value := strings.TrimSpace(filter.RunID); value != "" {
		query = query.Where("run_id = ?", value)
	}
	if value := strings.TrimSpace(filter.Kind); value != "" {
		query = query.Where("kind = ?", value)
	}
	return query
}

func applyStatusFilter(query *gorm.DB, status string) *gorm.DB {
	if value := strings.TrimSpace(status); value != "" {
		return query.Where("status = ?", value)
	}
	return query
}

func applyCreatedFilter(query *gorm.DB, from, to *time.Time) *gorm.DB {
	if from != nil {
		query = query.Where("created_at >= ?", *from)
	}
	if to != nil {
		query = query.Where("created_at <= ?", *to)
	}
	return query
}

func listPage[T any](query *gorm.DB, request PageRequest, label string) (Page[T], error) {
	pageRequest := request.Normalize()

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return Page[T]{}, fmt.Errorf("count admin %s: %w", label, err)
	}

	items := make([]T, 0)
	if err := query.
		Order("created_at DESC").
		Order("id DESC").
		Offset((pageRequest.Page - 1) * pageRequest.PageSize).
		Limit(pageRequest.PageSize).
		Find(&items).Error; err != nil {
		return Page[T]{}, fmt.Errorf("list admin %s: %w", label, err)
	}

	return Page[T]{Items: items, Page: pageRequest.Page, PageSize: pageRequest.PageSize, Total: total}, nil
}

func getByBusinessID[T any](query *gorm.DB, column, id, label string) (*T, error) {
	var entity T
	if err := query.Where(column+" = ?", strings.TrimSpace(id)).First(&entity).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("get admin %s: %w", label, err)
	}
	return &entity, nil
}
