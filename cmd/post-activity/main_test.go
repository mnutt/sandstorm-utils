package main

import (
	"context"
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
	if err == nil || !strings.Contains(err.Error(), "usage: post-activity [--timeout 10s] [--json-input FILE|-] [--path PATH] [--type N] [--thread-path PATH] [--thread-title TITLE] [--caption TEXT] <sessionId>") {
		t.Fatalf("unexpected error: %v", err)
	}
}
