package stayawake

import (
	"context"
	"testing"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestAcquireHeldLockRetainsHandleUntilExplicitRelease(t *testing.T) {
	t.Parallel()

	apiServer := &testcapnp.SandstormAPIServer{}
	lock, err := AcquireHeldLock(context.Background(), Options{
		Client: testcapnp.NewClient(&testcapnp.BridgeServer{APIServer: apiServer}),
	}, AcquireRequest{
		Title:   "Transcoding video",
		Caption: "Encoding in background",
	})
	if err != nil {
		t.Fatalf("AcquireHeldLock returned error: %v", err)
	}

	call := apiServer.LastStayAwakeCall()
	if call == nil {
		t.Fatal("expected stayAwake call")
	}
	if call.Caption != "Transcoding video: Encoding in background" {
		t.Fatalf("unexpected notification caption: %q", call.Caption)
	}

	assertNotReleased(t, call.HandleServer.ReleaseCh)

	if err := lock.Release(); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	waitForRelease(t, call.HandleServer.ReleaseCh)
}

func waitForRelease(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handle release")
	}
}

func assertNotReleased(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("handle was released unexpectedly")
	case <-time.After(100 * time.Millisecond):
	}
}
