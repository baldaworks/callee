package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const humanDuration = 43*time.Second + 453998585*time.Nanosecond

func TestInitConfiguresZerologAndSlog(t *testing.T) {
	t.Cleanup(func() { _ = Init(WithLevel(LevelInfo)) })

	if err := Init(WithLevel(LevelDebug)); err != nil {
		t.Fatal(err)
	}

	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Fatalf("zerolog level = %s", zerolog.GlobalLevel())
	}

	if zerolog.DurationFieldFormat != zerolog.DurationFormatString {
		t.Fatalf("zerolog duration format = %q, want string", zerolog.DurationFieldFormat)
	}

	if !slog.Default().Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("slog debug is disabled")
	}
}

func TestInitWithJSONWritesStructuredLogs(t *testing.T) {
	var output bytes.Buffer

	t.Cleanup(func() { _ = Init(WithLevel(LevelInfo)) })

	if err := Init(WithLevel(LevelInfo), WithJSON(true), WithWriter(&output)); err != nil {
		t.Fatal(err)
	}

	slog.Info("starting role", slog.String("role", "reviewer"), slog.Duration("duration", humanDuration))

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("unmarshal JSON log: %v\n%s", err, output.String())
	}

	if event["level"] != "info" || event["message"] != "starting role" || event["role"] != "reviewer" || event["duration"] != "43.453998585s" {
		t.Fatalf("event = %#v", event)
	}
}

func TestInitWritesHumanReadableConsoleDurations(t *testing.T) {
	var output bytes.Buffer

	t.Cleanup(func() { _ = Init(WithLevel(LevelInfo)) })

	if err := Init(WithLevel(LevelInfo), WithWriter(&output)); err != nil {
		t.Fatal(err)
	}

	log.Info().Dur("duration", humanDuration).Msg("agent finished")

	if got := stripANSI(output.String()); !strings.Contains(got, "duration=43.453998585s") {
		t.Fatalf("console output = %q, want human-readable duration", got)
	}
}

var ansiCSIPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(value string) string {
	return ansiCSIPattern.ReplaceAllString(value, "")
}

func TestJSONLineWriterWrapsCompleteAndPartialLines(t *testing.T) {
	var output bytes.Buffer

	writer := NewJSONLineWriter(&output)

	if _, err := writer.Write([]byte("first\r\nsecond")); err != nil {
		t.Fatal(err)
	}

	if _, err := writer.Write([]byte(" line\nthird")); err != nil {
		t.Fatal(err)
	}

	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}

	var events []map[string]any

	decoder := json.NewDecoder(&output)
	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}

		events = append(events, event)
	}

	wantMessages := []string{"first", "second line", "third"}
	if len(events) != len(wantMessages) {
		t.Fatalf("events = %#v", events)
	}

	for index, want := range wantMessages {
		if events[index]["level"] != "info" || events[index]["source"] != "provider" || events[index]["message"] != want {
			t.Fatalf("event %d = %#v", index, events[index])
		}
	}
}

func TestWriteJSONError(t *testing.T) {
	var output bytes.Buffer
	if err := WriteJSONError(&output, context.Canceled); err != nil {
		t.Fatal(err)
	}

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}

	if event["level"] != "error" || event["message"] != "command failed" || event["error"] != context.Canceled.Error() {
		t.Fatalf("event = %#v", event)
	}
}

func TestInitWithWarnSuppressesInfo(t *testing.T) {
	t.Cleanup(func() { _ = Init(WithLevel(LevelInfo)) })

	if err := Init(WithLevel(LevelWarn)); err != nil {
		t.Fatal(err)
	}

	if slog.Default().Handler().Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("slog info is enabled")
	}

	if !slog.Default().Handler().Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("slog warn is disabled")
	}
}

func TestInitRejectsInvalidLevel(t *testing.T) {
	if err := Init(WithLevel("invalid")); err == nil {
		t.Fatal("Init() error = nil")
	}
}

func TestZerologHandlerUsesConfiguredLogger(t *testing.T) {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(zerolog.InfoLevel) })

	var output bytes.Buffer

	logger := zerolog.New(zerolog.ConsoleWriter{Out: &output, NoColor: true})
	slog.New(newZerologHandler(logger)).Debug("starting acp process", slog.String("component", "acpagent.client"))

	got := output.String()
	if !strings.Contains(got, "DBG starting acp process") || !strings.Contains(got, "component=acpagent.client") {
		t.Fatalf("handler output = %q", got)
	}
}

func TestZerologHandlerRedactsACPPayloads(t *testing.T) {
	previousLevel := zerolog.GlobalLevel()

	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(previousLevel) })

	const secret = "do-not-log-this-acp-payload"

	var output bytes.Buffer

	logger := slog.New(newZerologHandler(zerolog.New(&output))).With(
		slog.String("component", acpSlogComponent),
		slog.String("subcomponent", "wire"),
	)

	logger.LogAttrs(context.Background(), slogTraceLevel, "acp wire",
		slog.String("direction", "outgoing"),
		slog.String("rpc_kind", "request"),
		slog.String("method", "session/prompt"),
		slog.String("id", "request-42"),
		slog.String("params", secret),
		slog.Any("result", json.RawMessage(`{"text":"do-not-log-this-acp-payload"}`)),
		slog.String("prompt", secret),
		slog.String("meta", secret),
		slog.String("raw_update", secret),
		slog.String("acp_update_payload", secret),
		slog.String("acp_content_block_text", secret),
		slog.Any("acp_content_block", map[string]string{"text": secret}),
		slog.String("error_message", secret),
	)

	if strings.Contains(output.String(), secret) {
		t.Fatalf("ACP trace output leaks payload: %s", output.String())
	}

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("unmarshal JSON log: %v\n%s", err, output.String())
	}

	if event["message"] != "acp wire" || event["direction"] != "outgoing" || event["rpc_kind"] != "request" || event["method"] != "session/prompt" || event["id"] != "request-42" {
		t.Fatalf("safe ACP diagnostics = %#v", event)
	}

	for _, field := range []string{
		"params", "result", "prompt", "meta", "raw_update", "acp_update_payload",
		"acp_content_block_text", "acp_content_block", "error_message",
	} {
		if _, ok := event[field]; ok {
			t.Errorf("event contains raw %s: %#v", field, event)
		}

		if _, ok := event[field+"_kind"]; !ok {
			t.Errorf("event is missing %s_kind: %#v", field, event)
		}

		if _, ok := event[field+"_bytes"]; !ok {
			t.Errorf("event is missing %s_bytes: %#v", field, event)
		}
	}

	if event["params_kind"] != "string" || event["params_bytes"] != float64(len(secret)) {
		t.Errorf("params metadata = %#v", event)
	}

	if event["result_kind"] != "json" {
		t.Errorf("result metadata = %#v", event)
	}
}

func TestZerologHandlerLeavesNonACPPayloadsUnchanged(t *testing.T) {
	previousLevel := zerolog.GlobalLevel()

	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(previousLevel) })

	const payload = "ordinary-log-payload"

	var output bytes.Buffer

	logger := slog.New(newZerologHandler(zerolog.New(&output))).With(
		slog.String("component", "callee.workflow"),
	)

	logger.LogAttrs(context.Background(), slogTraceLevel, "workflow trace", slog.String("payload", payload))

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("unmarshal JSON log: %v\n%s", err, output.String())
	}

	if event["payload"] != payload {
		t.Fatalf("non-ACP payload = %#v", event)
	}

	if _, ok := event["payload_kind"]; ok {
		t.Fatalf("non-ACP payload was redacted: %#v", event)
	}
}
