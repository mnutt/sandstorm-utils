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
	if err == nil || !strings.Contains(err.Error(), "usage: send-email [--timeout 10s] [--json-input FILE|-] [--from ADDR] [--from-name NAME] [--to ADDR] [--cc ADDR] [--bcc ADDR] [--reply-to ADDR] [--reply-to-name NAME] [--subject TEXT] [--text TEXT|--text-file FILE|-] [--html TEXT|--html-file FILE|-] <sessionId>") {
		t.Fatalf("unexpected error: %v", err)
	}
}
