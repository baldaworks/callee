// Package logging configures Callee's zerolog and slog loggers.
package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
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
	level  string
	json   bool
	writer io.Writer
}

// WithLevel selects trace, debug, info, warn, or error logging.
func WithLevel(level string) Option { return func(o *options) { o.level = level } }

// WithJSON writes JSON logs instead of human-readable console logs.
func WithJSON(enabled bool) Option { return func(o *options) { o.json = enabled } }

// WithWriter selects where logs are written. The default is standard error.
func WithWriter(writer io.Writer) Option { return func(o *options) { o.writer = writer } }

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

	zerolog.DurationFieldFormat = zerolog.DurationFormatString

	zerolog.SetGlobalLevel(zeroLevel)

	writer := opts.writer
	if writer == nil {
		writer = os.Stderr
	}

	writer = &synchronizedWriter{writer: writer}

	var logger zerolog.Logger
	if opts.json {
		logger = zerolog.New(writer)
	} else {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: writer, TimeFormat: time.RFC3339})
	}

	logger = logger.With().Timestamp().Logger()
	log.Logger = logger
	zerolog.DefaultContextLogger = &log.Logger

	_ = slogLevel

	slog.SetDefault(slog.New(newZerologHandler(logger)))

	return nil
}

type synchronizedWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *synchronizedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.writer.Write(data)
}

// WriteJSONError writes a command failure as one JSON Lines diagnostic event.
func WriteJSONError(writer io.Writer, err error) error {
	return json.NewEncoder(writer).Encode(diagnosticEvent{
		Level:   LevelError,
		Time:    time.Now().UTC(),
		Message: "command failed",
		Error:   err.Error(),
	})
}

// JSONLineWriter wraps raw diagnostic lines in JSON Lines events.
type JSONLineWriter struct {
	mu     sync.Mutex
	writer io.Writer
	buffer []byte
}

// NewJSONLineWriter returns an io.Writer that emits one event per input line.
func NewJSONLineWriter(writer io.Writer) *JSONLineWriter {
	return &JSONLineWriter{writer: writer}
}

// Write buffers incomplete lines and emits completed diagnostic events.
func (w *JSONLineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buffer = append(w.buffer, data...)
	for {
		index := bytes.IndexByte(w.buffer, '\n')
		if index < 0 {
			return len(data), nil
		}

		line := strings.TrimSuffix(string(w.buffer[:index]), "\r")

		w.buffer = w.buffer[index+1:]
		if err := w.writeEvent(line); err != nil {
			return len(data), err
		}
	}
}

// Flush emits the final unterminated line, if any.
func (w *JSONLineWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buffer) == 0 {
		return nil
	}

	line := strings.TrimSuffix(string(w.buffer), "\r")
	w.buffer = nil

	return w.writeEvent(line)
}

type diagnosticEvent struct {
	Level   string    `json:"level"`
	Time    time.Time `json:"time"`
	Source  string    `json:"source,omitempty"`
	Message string    `json:"message"`
	Error   string    `json:"error,omitempty"`
}

func (w *JSONLineWriter) writeEvent(message string) error {
	return json.NewEncoder(w.writer).Encode(diagnosticEvent{
		Level:   LevelInfo,
		Time:    time.Now().UTC(),
		Source:  "provider",
		Message: message,
	})
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
