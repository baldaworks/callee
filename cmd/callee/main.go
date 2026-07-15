package main

import (
	"context"
	"os"
	"os/signal"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	go func() {
		<-ctx.Done()
		stop()
	}()

	return ctx, stop
}
