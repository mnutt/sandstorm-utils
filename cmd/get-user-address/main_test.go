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
	if err == nil || !strings.Contains(err.Error(), "usage: get-user-address [--timeout 10s] [--json] <sessionId>") {
		t.Fatalf("unexpected error: %v", err)
	}
}
