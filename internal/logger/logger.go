package logger

import (
	"context"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	global *zap.Logger
	once   sync.Once
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func Init(level, format string) *zap.Logger {
	once.Do(func() {
		var cfg zap.Config
		if format == "text" {
			cfg = zap.NewDevelopmentConfig()
		} else {
			cfg = zap.NewProductionConfig()
		}

		switch level {
		case "debug":
			cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
		case "warn":
			cfg.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
		case "error":
			cfg.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
		default:
			cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		}

		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

		var err error
		global, err = cfg.Build(zap.AddCallerSkip(1))
		if err != nil {
			global, _ = zap.NewProduction()
		}
	})
	return global
}

func Get() *zap.Logger {
	if global == nil {
		global, _ = zap.NewProduction()
	}
	return global
}

func WithRequestID(ctx context.Context) *zap.Logger {
	l := Get()
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return l.With(zap.String("request_id", id))
	}
	return l
}

func WithContext(ctx context.Context, fields ...zap.Field) *zap.Logger {
	l := Get()
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		fields = append(fields, zap.String("request_id", id))
	}
	return l.With(fields...)
}

func Info(msg string, fields ...zap.Field)  { Get().Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { Get().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { Get().Error(msg, fields...) }
func Debug(msg string, fields ...zap.Field) { Get().Debug(msg, fields...) }

func Sync() { Get().Sync() }
