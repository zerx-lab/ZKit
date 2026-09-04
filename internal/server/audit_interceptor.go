package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/audit"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/clientip"
	"github.com/zerx-lab/zkit/internal/model"
)

// mutatingPrefixes are the procedure method-name prefixes considered mutating.
var mutatingPrefixes = []string{"Create", "Update", "Delete", "Set", "Sync", "Clean", "Revoke", "Logout", "Reorder"}

// opLogQueueSize bounds the number of operation logs waiting to be persisted.
const opLogQueueSize = 1024

// opLogWriter persists operation logs off the request path through a bounded
// queue drained by one goroutine, so a burst of RPCs can never fan out into an
// unbounded number of goroutines / DB writes. A full queue drops the record and
// logs it (audit must not become a DoS amplifier); Close drains the remainder.
type opLogWriter struct {
	db      *gorm.DB
	logger  *slog.Logger
	queue   chan model.OperationLog
	done    chan struct{}
	mu      sync.Mutex
	closed  bool
	dropped atomic.Int64
}

func newOpLogWriter(db *gorm.DB, logger *slog.Logger) *opLogWriter {
	w := &opLogWriter{
		db:     db,
		logger: logger,
		queue:  make(chan model.OperationLog, opLogQueueSize),
		done:   make(chan struct{}),
	}
	go w.run()

	return w
}

func (w *opLogWriter) run() {
	defer close(w.done)
	for rec := range w.queue {
		if err := gorm.G[model.OperationLog](w.db).Create(context.Background(), &rec); err != nil {
			w.logger.Error("operation log write failed",
				slog.String("procedure", rec.Procedure), slog.Any("err", err))
		}
	}
}

// enqueue hands rec to the writer without blocking; on overflow it is dropped.
func (w *opLogWriter) enqueue(rec model.OperationLog) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	select {
	case w.queue <- rec:
	default:
		n := w.dropped.Add(1)
		w.logger.Error("operation log dropped: queue full",
			slog.String("procedure", rec.Procedure), slog.Int64("dropped_total", n))
	}
}

// Close stops accepting records and waits for the queue to drain or ctx to end.
func (w *opLogWriter) Close(ctx context.Context) error {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.queue)
	}
	w.mu.Unlock()

	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewOperationLogInterceptor records an OperationLog for every mutating or
// failed RPC, and recovers handler panics (replacing connect.WithRecover so the
// panic and its stack are captured in the same log row). It is the sole writer
// of OperationLog.
func NewOperationLogInterceptor(w *opLogWriter) connect.UnaryInterceptorFunc {
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
					w.enqueue(buildOpLog(ctx, req, start, "panic", fmt.Sprint(p), string(debug.Stack()), holder.Detail))
				}
			}()

			resp, err = next(ctx, req)

			if !panicked && (isMutating(req.Spec().Procedure) || err != nil) {
				w.enqueue(buildOpLog(ctx, req, start, statusOf(err), errMsg(err), "", holder.Detail))
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

// buildOpLog assembles the record synchronously in the request goroutine (so
// context values are still valid); it never includes the request body.
func buildOpLog(ctx context.Context, req connect.AnyRequest, start time.Time, status, errStr, stack, detail string) model.OperationLog {
	rec := model.OperationLog{
		CreatedAt: time.Now(),
		Procedure: req.Spec().Procedure,
		Method:    methodName(req.Spec().Procedure),
		IP:        clientip.Of(ctx, req),
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

	return rec
}
