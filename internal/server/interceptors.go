package server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"github.com/zerx-lab/zkit/internal/clientip"
	"github.com/zerx-lab/zkit/internal/ratelimit"
)

// NewLoggingInterceptor logs every unary RPC with its request id, procedure,
// duration, and (on failure) connect error code plus the unsanitized message.
func NewLoggingInterceptor(logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			res, err := next(ctx, req)

			if err != nil {
				logger.LogAttrs(ctx, slog.LevelWarn, "rpc failed",
					slog.String("request_id", requestIDFrom(ctx)),
					slog.String("procedure", req.Spec().Procedure),
					slog.Duration("duration", time.Since(start)),
					slog.String("code", connect.CodeOf(err).String()),
					slog.String("err", err.Error()),
				)
			} else {
				logger.LogAttrs(ctx, slog.LevelInfo, "rpc ok",
					slog.String("request_id", requestIDFrom(ctx)),
					slog.String("procedure", req.Spec().Procedure),
					slog.Duration("duration", time.Since(start)),
				)
			}

			return res, err
		}
	}
}

// NewRateLimitInterceptor rejects requests exceeding the per-IP rate limit with
// CodeResourceExhausted. Placed outside the operation-log interceptor so
// rejected requests are not persisted (avoiding DB amplification under load).
func NewRateLimitInterceptor(l *ratelimit.Limiter) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if !l.Allow(clientip.Of(ctx, req)) {
				return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("请求过于频繁"))
			}

			return next(ctx, req)
		}
	}
}
