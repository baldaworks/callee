package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
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

	if got := output.String(); !strings.Contains(got, "duration=43.453998585s") {
		t.Fatalf("console output = %q, want human-readable duration", got)
	}
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
