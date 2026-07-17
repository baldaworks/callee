package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/baldaworks/callee/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := commandContext()
	defer stop()

	return cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

func commandContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(context.Background())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	var once sync.Once

	stop := func() {
		once.Do(func() {
			signal.Stop(signals)
			cancel(context.Canceled)
		})
	}

	go func() {
		var selected os.Signal
		select {
		case selected = <-signals:
		case <-ctx.Done():
			return
		}

		signal.Stop(signals)

		switch selected {
		case os.Interrupt:
			cancel(cli.ErrInterrupt)
		case syscall.SIGTERM:
			cancel(cli.ErrTerminate)
		default:
			cancel(context.Canceled)
		}
	}()

	return ctx, stop
}
