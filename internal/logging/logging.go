// Package logging configures Callee's zerolog and slog loggers.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	slogTraceLevel = slog.LevelDebug - 4
	LevelTrace     = "trace"
	LevelDebug     = "debug"
	LevelInfo      = "info"
	LevelWarn      = "warn"
	LevelError     = "error"
)

// Option configures application logging.
type Option func(*options)

type options struct {
	level string
	json  bool
}

// WithLevel selects trace, debug, info, warn, or error logging.
func WithLevel(level string) Option { return func(o *options) { o.level = level } }

// WithJSON writes JSON logs instead of human-readable console logs.
func WithJSON(enabled bool) Option { return func(o *options) { o.json = enabled } }

// Init configures zerolog as Callee's logger and slog for dependencies.
func Init(setters ...Option) error {
	opts := options{level: LevelInfo}
	for _, set := range setters {
		set(&opts)
	}

	zeroLevel, slogLevel, err := resolveLevel(opts.level)
	if err != nil {
		return err
	}

	zerolog.SetGlobalLevel(zeroLevel)

	var logger zerolog.Logger
	if opts.json {
		logger = zerolog.New(os.Stderr)
	} else {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	logger = logger.With().Timestamp().Logger()
	log.Logger = logger
	zerolog.DefaultContextLogger = &log.Logger

	_ = slogLevel

	slog.SetDefault(slog.New(newZerologHandler(logger)))

	return nil
}

func resolveLevel(raw string) (zerolog.Level, slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", LevelInfo:
		return zerolog.InfoLevel, slog.LevelInfo, nil
	case LevelTrace:
		return zerolog.TraceLevel, slogTraceLevel, nil
	case LevelDebug:
		return zerolog.DebugLevel, slog.LevelDebug, nil
	case LevelWarn, "warning":
		return zerolog.WarnLevel, slog.LevelWarn, nil
	case LevelError:
		return zerolog.ErrorLevel, slog.LevelError, nil
	default:
		return zerolog.NoLevel, slog.LevelInfo, fmt.Errorf("resolve logging level: unsupported level %q (allowed: trace, debug, info, warn, error)", raw)
	}
}

type zerologHandler struct {
	logger zerolog.Logger
	attrs  []slog.Attr
	groups []string
}

func newZerologHandler(logger zerolog.Logger) slog.Handler {
	return &zerologHandler{logger: logger}
}

func (h *zerologHandler) Enabled(_ context.Context, level slog.Level) bool {
	return slogLevelToZerolog(level) >= zerolog.GlobalLevel()
}

func (h *zerologHandler) Handle(ctx context.Context, record slog.Record) error {
	if !h.Enabled(ctx, record.Level) {
		return nil
	}

	event := h.logger.WithLevel(slogLevelToZerolog(record.Level))
	for _, attr := range h.attrs {
		event = appendSlogAttr(event, h.groups, attr)
	}

	record.Attrs(func(attr slog.Attr) bool {
		event = appendSlogAttr(event, h.groups, attr)

		return true
	})
	event.Msg(record.Message)

	return nil
}

func (h *zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)

	return &clone
}

func (h *zerologHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)

	return &clone
}

func slogLevelToZerolog(level slog.Level) zerolog.Level {
	switch {
	case level <= slogTraceLevel:
		return zerolog.TraceLevel
	case level < slog.LevelInfo:
		return zerolog.DebugLevel
	case level < slog.LevelWarn:
		return zerolog.InfoLevel
	case level < slog.LevelError:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}

func appendSlogAttr(event *zerolog.Event, groups []string, attr slog.Attr) *zerolog.Event {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			groups = append(groups, attr.Key)
		}

		for _, groupAttr := range attr.Value.Group() {
			event = appendSlogAttr(event, groups, groupAttr)
		}

		return event
	}

	if attr.Key == "" {
		return event
	}

	key := strings.Join(append(append([]string(nil), groups...), attr.Key), ".")
	switch attr.Value.Kind() {
	case slog.KindString:
		return event.Str(key, attr.Value.String())
	case slog.KindBool:
		return event.Bool(key, attr.Value.Bool())
	case slog.KindInt64:
		return event.Int64(key, attr.Value.Int64())
	case slog.KindUint64:
		return event.Uint64(key, attr.Value.Uint64())
	case slog.KindFloat64:
		return event.Float64(key, attr.Value.Float64())
	case slog.KindDuration:
		return event.Dur(key, attr.Value.Duration())
	case slog.KindTime:
		return event.Time(key, attr.Value.Time())
	case slog.KindAny:
		if err, ok := attr.Value.Any().(error); ok {
			return event.AnErr(key, err)
		}

		return event.Interface(key, attr.Value.Any())
	default:
		return event.Interface(key, attr.Value.Any())
	}
}
