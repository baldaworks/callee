package workflow

import (
	"context"
	"time"

	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
)

const defaultTurnHeartbeatInterval = 10 * time.Second

var (
	turnHeartbeatInterval  = defaultTurnHeartbeatInterval
	turnHeartbeatNow       = time.Now
	newTurnHeartbeatTicker = func(interval time.Duration) turnHeartbeatTicker {
		return realTurnHeartbeatTicker{Ticker: time.NewTicker(interval)}
	}
)

type turnHeartbeatTicker interface {
	Chan() <-chan time.Time
	Stop()
}

type realTurnHeartbeatTicker struct {
	*time.Ticker
}

func (t realTurnHeartbeatTicker) Chan() <-chan time.Time {
	return t.C
}

func runTurnWithHeartbeat(
	ctx context.Context,
	logger zerolog.Logger,
	session runtime.AgentSession,
	prompt string,
) (runtime.TurnResult, error) {
	if turnHeartbeatInterval <= 0 {
		return session.Turn(ctx, prompt)
	}

	started := turnHeartbeatNow()

	ticker := newTurnHeartbeatTicker(turnHeartbeatInterval)
	defer ticker.Stop()

	type turnOutcome struct {
		result runtime.TurnResult
		err    error
	}

	done := make(chan turnOutcome, 1)

	go func() {
		result, err := session.Turn(ctx, prompt)
		done <- turnOutcome{result: result, err: err}
	}()

	for {
		select {
		case outcome := <-done:
			return outcome.result, outcome.err
		case <-ticker.Chan():
			select {
			case outcome := <-done:
				return outcome.result, outcome.err
			default:
			}

			logger.Info().Dur("turn_duration", turnHeartbeatNow().Sub(started)).Msg("agent turn heartbeat")
		}
	}
}
