package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime/debug"
	"strings"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/model"
)

// mutatingPrefixes are the procedure method-name prefixes considered mutating.
var mutatingPrefixes = []string{"Create", "Update", "Delete", "Set", "Sync", "Clean", "Revoke", "Logout", "Reorder"}

// NewOperationLogInterceptor records an OperationLog for every mutating or
// failed RPC, and recovers handler panics (replacing connect.WithRecover so the
// panic and its stack are captured in the same log row). It is the sole writer
// of OperationLog.
func NewOperationLogInterceptor(db *gorm.DB) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
			start := time.Now()
			panicked := false
			ctx, holder := audit.WithHolder(ctx)

			defer func() {
				if p := recover(); p != nil {
					panicked = true
					err = connect.NewError(connect.CodeInternal, errors.New("internal error"))
					resp = nil
					writeOpLog(ctx, db, req, start, "panic", fmt.Sprint(p), string(debug.Stack()), holder.Detail)
				}
			}()

			resp, err = next(ctx, req)

			if !panicked && (isMutating(req.Spec().Procedure) || err != nil) {
				writeOpLog(ctx, db, req, start, statusOf(err), errMsg(err), "", holder.Detail)
			}

			return resp, err
		}
	}
}

// methodName returns the trailing segment of a connectRPC procedure path.
func methodName(procedure string) string {
	if i := strings.LastIndex(procedure, "/"); i >= 0 {
		return procedure[i+1:]
	}

	return procedure
}

func isMutating(procedure string) bool {
	m := methodName(procedure)
	for _, p := range mutatingPrefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}

	return false
}

func statusOf(err error) string {
	if err == nil {
		return "ok"
	}

	return connect.CodeOf(err).String()
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

func auditClientIP(req connect.AnyRequest) string {
	addr := req.Peer().Addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}

	return addr
}

// writeOpLog persists an operation log asynchronously (never blocks the request,
// never records request bodies).
func writeOpLog(ctx context.Context, db *gorm.DB, req connect.AnyRequest, start time.Time, status, errStr, stack, detail string) {
	rec := model.OperationLog{
		CreatedAt: time.Now(),
		Procedure: req.Spec().Procedure,
		Method:    methodName(req.Spec().Procedure),
		IP:        auditClientIP(req),
		UserAgent: req.Header().Get("User-Agent"),
		LatencyMS: time.Since(start).Milliseconds(),
		Status:    status,
		Error:     errStr,
		Stack:     stack,
		Detail:    detail,
	}
	if claims, ok := auth.ClaimsFromContext(ctx); ok && claims != nil {
		rec.UserID = claims.UserID
	}

	go func() {
		_ = gorm.G[model.OperationLog](db).Create(context.Background(), &rec)
	}()
}
