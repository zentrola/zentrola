package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func optionalLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}
	return discardLogger()
}

// diagnosePostgresError 记录可诊断的数据库错误，同时只向上层返回稳定的应用错误。
// 日志刻意不包含 SQL 参数、连接串或凭据。
func diagnosePostgresError(ctx context.Context, logger *slog.Logger, component, operation string, cause, public error) error {
	if cause == nil {
		return nil
	}
	if errors.Is(cause, public) {
		return public
	}
	attributes := []any{
		"component", component,
		"operation", operation,
		"cause_type", fmt.Sprintf("%T", cause),
	}
	var pgErr *pgconn.PgError
	if errors.As(cause, &pgErr) {
		attributes = append(attributes,
			"sqlstate", pgErr.Code,
			"constraint", pgErr.ConstraintName,
		)
	} else if errors.Is(cause, context.Canceled) {
		attributes = append(attributes, "cause_code", "context_canceled")
	} else if errors.Is(cause, context.DeadlineExceeded) {
		attributes = append(attributes, "cause_code", "deadline_exceeded")
	}
	logger.ErrorContext(ctx, "PostgreSQL operation failed", attributes...)
	return public
}
