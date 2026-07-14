package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestInitConfiguresZerologAndSlog(t *testing.T) {
	t.Cleanup(func() { _ = Init(WithLevel(LevelInfo)) })

	if err := Init(WithLevel(LevelDebug)); err != nil {
		t.Fatal(err)
	}

	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Fatalf("zerolog level = %s", zerolog.GlobalLevel())
	}

	if !slog.Default().Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("slog debug is disabled")
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
