package service

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "github.com/zerx-lab/zkit/gen/go/zerx/v1"
	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/model"
)

// DashboardService implements zerxv1connect.DashboardServiceHandler.
// Authorization is enforced by the Casbin interceptor.
type DashboardService struct {
	db *gorm.DB
}

var _ zerxv1connect.DashboardServiceHandler = (*DashboardService)(nil)

// NewDashboardService constructs the dashboard handler.
func NewDashboardService(db *gorm.DB) *DashboardService {
	return &DashboardService{db: db}
}

func (s *DashboardService) GetDashboardStats(ctx context.Context, _ *connect.Request[zerxv1.GetDashboardStatsRequest]) (*connect.Response[zerxv1.GetDashboardStatsResponse], error) {
	totalUsers, err := gorm.G[model.User](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	totalRoles, err := gorm.G[model.Role](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	activeSessions, err := gorm.G[model.UserSession](s.db).Where("expires_at > ?", time.Now()).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	startOfDay := time.Now().Truncate(24 * time.Hour)
	todayLogins, err := gorm.G[model.LoginLog](s.db).Where("created_at >= ?", startOfDay).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	const days = 14
	since := time.Now().AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)

	users, err := gorm.G[model.User](s.db).Where("created_at >= ?", since).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	logins, err := gorm.G[model.LoginLog](s.db).Where("created_at >= ?", since).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ops, err := gorm.G[model.OperationLog](s.db).Where("created_at >= ?", since).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	userBuckets := newBuckets(since, days)
	loginOK := newBuckets(since, days)
	loginFail := newBuckets(since, days)
	opBuckets := newBuckets(since, days)
	for i := range users {
		userBuckets.add(users[i].CreatedAt, 1)
	}
	for i := range logins {
		if logins[i].Success {
			loginOK.add(logins[i].CreatedAt, 1)
		} else {
			loginFail.add(logins[i].CreatedAt, 1)
		}
	}
	for i := range ops {
		opBuckets.add(ops[i].CreatedAt, 1)
	}

	return connect.NewResponse(&zerxv1.GetDashboardStatsResponse{
		TotalUsers:     totalUsers,
		TotalRoles:     totalRoles,
		ActiveSessions: activeSessions,
		TodayLogins:    todayLogins,
		UserGrowth:     userBuckets.points(),
		LoginSuccess:   loginOK.points(),
		LoginFailure:   loginFail.points(),
		OperationCount: opBuckets.points(),
	}), nil
}

// buckets groups daily counts over a fixed window for time-series output.
type buckets struct {
	start  time.Time
	counts []int64
	dates  []string
}

func newBuckets(start time.Time, days int) *buckets {
	b := &buckets{start: start, counts: make([]int64, days), dates: make([]string, days)}
	for i := 0; i < days; i++ {
		b.dates[i] = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	return b
}

func (b *buckets) add(t time.Time, n int64) {
	idx := int(t.Truncate(24*time.Hour).Sub(b.start).Hours() / 24)
	if idx >= 0 && idx < len(b.counts) {
		b.counts[idx] += n
	}
}

func (b *buckets) points() []*zerxv1.TimePoint {
	out := make([]*zerxv1.TimePoint, 0, len(b.counts))
	for i := range b.counts {
		out = append(out, &zerxv1.TimePoint{Date: b.dates[i], Value: b.counts[i]})
	}
	return out
}
