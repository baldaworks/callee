//go:build linux || darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

const signalHelper = "CALLEE_SIGNAL_HELPER"

func TestCommandContextRestoresDefaultSignalHandling(t *testing.T) {
	if os.Getenv(signalHelper) == "1" {
		ctx, stop := commandContext()
		defer stop()

		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			os.Exit(90)
		}

		if err := process.Signal(os.Interrupt); err != nil {
			os.Exit(91)
		}

		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)

		_, _ = fmt.Fprintln(os.Stdout, "first signal handled")

		if err := process.Signal(os.Interrupt); err != nil {
			os.Exit(92)
		}

		time.Sleep(time.Second)
		os.Exit(93)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCommandContextRestoresDefaultSignalHandling$")

	cmd.Env = append(os.Environ(), signalHelper+"=1")

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helper survived second signal; output = %q", output)
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("helper error = %T %v", err, err)
	}

	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != os.Interrupt {
		t.Fatalf("helper status = %v, want interrupt signal; output = %q", exitErr.Sys(), output)
	}

	if !strings.Contains(string(output), "first signal handled") {
		t.Fatalf("helper output = %q, first signal was not handled", output)
	}
}
