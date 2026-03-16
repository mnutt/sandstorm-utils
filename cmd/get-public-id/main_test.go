package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	t.Parallel()

	old := os.Args
	os.Args = []string{commandName}
	t.Cleanup(func() { os.Args = old })

	err := run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "usage: get-public-id [--timeout 10s] [--json] <sessionId>") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHelpOutput(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	done := make(chan struct{})
	go func() {
		_, _ = stderr.ReadFrom(r)
		close(done)
	}()

	err = run(context.Background(), []string{"-h"})
	_ = w.Close()
	<-done

	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}

	output := stderr.String()
	if !strings.Contains(output, commandPurpose) {
		t.Fatalf("missing command purpose in help output: %q", output)
	}
	if !strings.Contains(output, "Usage:\n  get-public-id [--timeout 10s] [--json] <sessionId>") {
		t.Fatalf("missing usage in help output: %q", output)
	}
	if !strings.Contains(output, "-json") || !strings.Contains(output, "-timeout") {
		t.Fatalf("missing flags in help output: %q", output)
	}
}
