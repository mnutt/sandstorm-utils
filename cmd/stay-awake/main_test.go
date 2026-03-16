package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "usage: stay-awake <acquire|renew|release> [flags]") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcquireRequiresSessionID(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"acquire", "--title", "Transcoding"})
	if err == nil || !strings.Contains(err.Error(), "usage: stay-awake acquire") {
		t.Fatalf("unexpected error: %v", err)
	}
}
