package service

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/jobs"
	"github.com/zerx-lab/zkit/internal/model"
	"github.com/zerx-lab/zkit/internal/plugin"
)

// JobService implements zerxv1connect.JobServiceHandler. Authorization is
// enforced by the Casbin interceptor.
type JobService struct {
	db          *gorm.DB
	scheduler   *jobs.Scheduler
	registry    jobs.Registry
	pluginState *plugin.State
}

var _ zerxv1connect.JobServiceHandler = (*JobService)(nil)

// NewJobService constructs the job handler. scheduler may be nil in tests that
// never call scheduling RPCs.
func NewJobService(db *gorm.DB, scheduler *jobs.Scheduler, registry jobs.Registry, pluginState *plugin.State) *JobService {
	return &JobService{db: db, scheduler: scheduler, registry: registry, pluginState: pluginState}
}

// handlerEnabled reports whether a job handler key may be listed/scheduled: core
// handlers always, plugin handlers only when the owning plugin is enabled. A nil
// pluginState (tests) treats all as enabled.
func (s *JobService) handlerEnabled(key string) bool {
	if s.pluginState == nil {
		return true
	}
	return s.pluginState.IsJobHandlerEnabled(key)
}

func (s *JobService) ListJobs(ctx context.Context, req *connect.Request[zerxv1.ListJobsRequest]) (*connect.Response[zerxv1.ListJobsResponse], error) {
	_, ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	total, err := gorm.G[model.ScheduledJob](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[model.ScheduledJob](s.db).Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*zerxv1.Job, 0, len(rows))
	for i := range rows {
		out = append(out, toProtoJob(rows[i]))
	}

	return connect.NewResponse(&zerxv1.ListJobsResponse{Jobs: out, Total: total}), nil
}

func (s *JobService) CreateJob(ctx context.Context, req *connect.Request[zerxv1.CreateJobRequest]) (*connect.Response[zerxv1.Job], error) {
	if err := s.validate(req.Msg.GetHandler(), req.Msg.GetCronExpr()); err != nil {
		return nil, err
	}
	job := model.ScheduledJob{
		Name:        req.Msg.GetName(),
		Handler:     req.Msg.GetHandler(),
		CronExpr:    req.Msg.GetCronExpr(),
		Enabled:     req.Msg.GetEnabled(),
		Description: req.Msg.GetDescription(),
	}
	if err := gorm.G[model.ScheduledJob](s.db).Create(ctx, &job); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if s.scheduler != nil {
		if err := s.scheduler.Reschedule(job); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"name": job.Name, "handler": job.Handler, "cron": job.CronExpr, "enabled": job.Enabled}}))

	return connect.NewResponse(toProtoJob(job)), nil
}

func (s *JobService) UpdateJob(ctx context.Context, req *connect.Request[zerxv1.UpdateJobRequest]) (*connect.Response[zerxv1.Job], error) {
	id := req.Msg.GetId()
	before, err := gorm.G[model.ScheduledJob](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("job not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.validate(req.Msg.GetHandler(), req.Msg.GetCronExpr()); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"name":        req.Msg.GetName(),
		"handler":     req.Msg.GetHandler(),
		"cron_expr":   req.Msg.GetCronExpr(),
		"enabled":     req.Msg.GetEnabled(),
		"description": req.Msg.GetDescription(),
	}
	if err := s.db.WithContext(ctx).Model(&model.ScheduledJob{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	job, err := gorm.G[model.ScheduledJob](s.db).Where("id = ?", id).First(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if s.scheduler != nil {
		// Remove the old name's schedule (in case the name changed), then apply.
		s.scheduler.Remove(before.Name)
		if err := s.scheduler.Reschedule(job); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	audit.Record(ctx, auditJSON(map[string]any{
		"before": map[string]any{"name": before.Name, "cron": before.CronExpr, "enabled": before.Enabled},
		"after":  map[string]any{"name": job.Name, "cron": job.CronExpr, "enabled": job.Enabled},
	}))

	return connect.NewResponse(toProtoJob(job)), nil
}

func (s *JobService) DeleteJob(ctx context.Context, req *connect.Request[zerxv1.DeleteJobRequest]) (*connect.Response[zerxv1.DeleteJobResponse], error) {
	job, err := gorm.G[model.ScheduledJob](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("job not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := gorm.G[model.ScheduledJob](s.db).Where("id = ?", job.ID).Delete(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if s.scheduler != nil {
		s.scheduler.Remove(job.Name)
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"name": job.Name}}))

	return connect.NewResponse(&zerxv1.DeleteJobResponse{}), nil
}

func (s *JobService) RunJobNow(ctx context.Context, req *connect.Request[zerxv1.RunJobNowRequest]) (*connect.Response[zerxv1.RunJobNowResponse], error) {
	job, err := gorm.G[model.ScheduledJob](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("job not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if s.scheduler != nil {
		if err := s.scheduler.RunNow(job.ID, job.Name, job.Handler); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	return connect.NewResponse(&zerxv1.RunJobNowResponse{}), nil
}

func (s *JobService) ListJobExecutions(ctx context.Context, req *connect.Request[zerxv1.ListJobExecutionsRequest]) (*connect.Response[zerxv1.ListJobExecutionsResponse], error) {
	_, ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	jobID := req.Msg.GetJobId()
	total, err := gorm.G[model.JobExecution](s.db).Where("job_id = ?", jobID).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[model.JobExecution](s.db).Where("job_id = ?", jobID).Order("id DESC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*zerxv1.JobExecution, 0, len(rows))
	for i := range rows {
		out = append(out, toProtoJobExecution(rows[i]))
	}

	return connect.NewResponse(&zerxv1.ListJobExecutionsResponse{Executions: out, Total: total}), nil
}

func (s *JobService) ListHandlers(_ context.Context, _ *connect.Request[zerxv1.ListHandlersRequest]) (*connect.Response[zerxv1.ListHandlersResponse], error) {
	out := make([]*zerxv1.JobHandler, 0, len(s.registry))
	for key, desc := range s.registry {
		if !s.handlerEnabled(key) {
			continue // hide handlers of disabled plugins from the picker
		}
		out = append(out, &zerxv1.JobHandler{Key: key, Description: desc.Description})
	}

	return connect.NewResponse(&zerxv1.ListHandlersResponse{Handlers: out}), nil
}

func (s *JobService) validate(handler, cronExpr string) error {
	if _, ok := s.registry[handler]; !ok {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("未知的任务处理器"))
	}
	if !s.handlerEnabled(handler) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("该任务处理器所属插件已禁用"))
	}
	if !jobs.ValidCron(cronExpr) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("无效的 cron 表达式"))
	}
	return nil
}

func toProtoJob(j model.ScheduledJob) *zerxv1.Job {
	lastRun := ""
	if j.LastRunAt != nil {
		lastRun = j.LastRunAt.Format(time.RFC3339)
	}
	return &zerxv1.Job{
		Id:          j.ID,
		Name:        j.Name,
		Handler:     j.Handler,
		CronExpr:    j.CronExpr,
		Enabled:     j.Enabled,
		Description: j.Description,
		LastRunAt:   lastRun,
		CreatedAt:   j.CreatedAt.Format(time.RFC3339),
	}
}

func toProtoJobExecution(e model.JobExecution) *zerxv1.JobExecution {
	return &zerxv1.JobExecution{
		Id:         e.ID,
		JobId:      e.JobID,
		StartedAt:  e.StartedAt.Format(time.RFC3339),
		FinishedAt: e.FinishedAt.Format(time.RFC3339),
		Status:     e.Status,
		Error:      e.Error,
		DurationMs: e.DurationMS,
	}
}
