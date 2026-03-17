package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "usage: stay-awake [--timeout 10s]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequiresSessionID(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"--title", "Transcoding"})
	if err == nil || !strings.Contains(err.Error(), "usage: stay-awake [--timeout 10s]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsNegativeHoldDuration(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"--for", "-1s", "--title", "Transcoding", "session-1"})
	if err == nil || !strings.Contains(err.Error(), "--for must be non-negative") {
		t.Fatalf("unexpected error: %v", err)
	}
}
