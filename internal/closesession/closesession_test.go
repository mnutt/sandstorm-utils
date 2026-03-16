package closesession

import (
	"context"
	"testing"

	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestRunWithClient(t *testing.T) {
	t.Parallel()

	sessionServer := &testcapnp.SessionServer{}
	client := testcapnp.NewClient(&testcapnp.BridgeServer{SessionServer: sessionServer})

	if err := RunWithClient(context.Background(), client, "session-1"); err != nil {
		t.Fatalf("RunWithClient returned error: %v", err)
	}

	if sessionServer.CloseCalls != 1 {
		t.Fatalf("expected 1 close call, got %d", sessionServer.CloseCalls)
	}
}
