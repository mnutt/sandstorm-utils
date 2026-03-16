package openview

import (
	"context"
	"testing"

	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestRunWithClient(t *testing.T) {
	t.Parallel()

	sessionServer := &testcapnp.SessionServer{}
	client := testcapnp.NewClient(&testcapnp.BridgeServer{SessionServer: sessionServer})

	err := RunWithClient(context.Background(), client, "session-1", Options{
		Path:   "/docs/123",
		NewTab: true,
	})
	if err != nil {
		t.Fatalf("RunWithClient returned error: %v", err)
	}

	if len(sessionServer.OpenViewCalls) != 1 {
		t.Fatalf("expected 1 openView call, got %d", len(sessionServer.OpenViewCalls))
	}

	call := sessionServer.OpenViewCalls[0]
	if call.Path != "docs/123" || !call.NewTab {
		t.Fatalf("unexpected openView call: %+v", call)
	}
}
