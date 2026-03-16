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
	if err == nil || !strings.Contains(err.Error(), "usage: open-view [--timeout 10s] [--path PATH] [--new-tab] <sessionId>") {
		t.Fatalf("unexpected error: %v", err)
	}
}
