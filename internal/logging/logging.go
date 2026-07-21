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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	slogTraceLevel                = slog.LevelDebug - 4
	acpSlogComponent              = "runtime.agentfactory.acp"
	acpRedactedFieldMetadataCount = 2
	LevelTrace                    = "trace"
	LevelDebug                    = "debug"
	LevelInfo                     = "info"
	LevelWarn                     = "warn"
	LevelError                    = "error"
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
	attrs := flattenSlogAttrs(h.groups, h.attrs)

	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, flattenSlogAttrs(h.groups, []slog.Attr{attr})...)

		return true
	})

	if isACPLogEvent(attrs) {
		attrs = redactACPPayloads(attrs)
	}

	for _, attr := range attrs {
		event = appendSlogField(event, attr)
	}

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

type slogField struct {
	key   string
	value slog.Value
}

func flattenSlogAttrs(groups []string, attrs []slog.Attr) []slogField {
	fields := make([]slogField, 0, len(attrs))
	for _, attr := range attrs {
		fields = appendSlogAttrFields(fields, groups, attr)
	}

	return fields
}

func appendSlogAttrFields(fields []slogField, groups []string, attr slog.Attr) []slogField {
	value := attr.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		if attr.Key != "" {
			groups = append(append([]string(nil), groups...), attr.Key)
		}

		for _, groupAttr := range value.Group() {
			fields = appendSlogAttrFields(fields, groups, groupAttr)
		}

		return fields
	}

	if attr.Key == "" {
		return fields
	}

	return append(fields, slogField{
		key:   strings.Join(append(append([]string(nil), groups...), attr.Key), "."),
		value: value,
	})
}

func appendSlogField(event *zerolog.Event, field slogField) *zerolog.Event {
	switch field.value.Kind() {
	case slog.KindString:
		return event.Str(field.key, field.value.String())
	case slog.KindBool:
		return event.Bool(field.key, field.value.Bool())
	case slog.KindInt64:
		return event.Int64(field.key, field.value.Int64())
	case slog.KindUint64:
		return event.Uint64(field.key, field.value.Uint64())
	case slog.KindFloat64:
		return event.Float64(field.key, field.value.Float64())
	case slog.KindDuration:
		return event.Dur(field.key, field.value.Duration())
	case slog.KindTime:
		return event.Time(field.key, field.value.Time())
	case slog.KindAny:
		if err, ok := field.value.Any().(error); ok {
			return event.AnErr(field.key, err)
		}

		return event.Interface(field.key, field.value.Any())
	default:
		return event.Interface(field.key, field.value.Any())
	}
}

func isACPLogEvent(fields []slogField) bool {
	for _, field := range fields {
		if field.key == "component" && field.value.Kind() == slog.KindString && field.value.String() == acpSlogComponent {
			return true
		}
	}

	return false
}

func redactACPPayloads(fields []slogField) []slogField {
	redacted := make([]slogField, 0, len(fields)*acpRedactedFieldMetadataCount)
	for _, field := range fields {
		if isSafeACPField(field.key) {
			redacted = append(redacted, field)

			continue
		}

		redacted = append(redacted,
			slogField{key: field.key + "_kind", value: slog.StringValue(slogFieldKind(field.value))},
			slogField{key: field.key + "_bytes", value: slog.Int64Value(slogFieldByteCount(field.value))},
		)
	}

	return redacted
}

func isSafeACPField(key string) bool {
	switch key {
	case "component", "subcomponent",
		"direction", "rpc_kind", "method", "id",
		"acp_session_id", "adk_session_id", "invocation_id",
		"update_kind", "acp_update_type", "acp_content_block_type", "acp_payload_type",
		"prompt_blocks", "prompt_len", "session_config_values", "option_count",
		"partial", "thought", "last_in_series", "has_meta", "has_content", "turn_complete",
		"finish_reason", "error_code", "protocol_version", "pid", "method_id", "option_id", "option_kind":
		return true
	default:
		return false
	}
}

func slogFieldKind(value slog.Value) string {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return "string"
	case slog.KindBool:
		return "bool"
	case slog.KindInt64:
		return "int64"
	case slog.KindUint64:
		return "uint64"
	case slog.KindFloat64:
		return "float64"
	case slog.KindDuration:
		return "duration"
	case slog.KindTime:
		return "time"
	case slog.KindAny:
		switch value.Any().(type) {
		case nil:
			return "null"
		case json.RawMessage:
			return "json"
		case []byte:
			return "bytes"
		case error:
			return "error"
		default:
			return "any"
		}
	default:
		return "unknown"
	}
}

func slogFieldByteCount(value slog.Value) int64 {
	value = value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return int64(len(value.String()))
	case slog.KindBool:
		return int64(len(strconv.FormatBool(value.Bool())))
	case slog.KindInt64:
		return int64(len(strconv.FormatInt(value.Int64(), 10)))
	case slog.KindUint64:
		return int64(len(strconv.FormatUint(value.Uint64(), 10)))
	case slog.KindFloat64:
		return int64(len(strconv.FormatFloat(value.Float64(), 'g', -1, 64)))
	case slog.KindDuration:
		return int64(len(value.Duration().String()))
	case slog.KindTime:
		return int64(len(value.Time().Format(time.RFC3339Nano)))
	case slog.KindAny:
		return anyByteCount(value.Any())
	default:
		return -1
	}
}

func anyByteCount(value any) int64 {
	switch raw := value.(type) {
	case nil:
		return 0
	case string:
		return int64(len(raw))
	case json.RawMessage:
		return int64(len(raw))
	case []byte:
		return int64(len(raw))
	case error:
		return int64(len(raw.Error()))
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return -1
		}

		return int64(len(encoded))
	}
}
