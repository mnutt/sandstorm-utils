package stayawake

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestBrokerAcquireRenewRelease(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 15, 4, 5, 0, time.UTC)
	apiServer := &testcapnp.SandstormAPIServer{}
	broker := NewBroker(Options{
		Client: testcapnp.NewClient(&testcapnp.BridgeServer{APIServer: apiServer}),
		Now:    func() time.Time { return now },
	})

	resp, err := broker.Acquire(context.Background(), AcquireRequest{
		SessionID: "session-1",
		TTL:       2 * time.Minute,
		Title:     "Transcoding video",
		Caption:   "Encoding in background",
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	call := apiServer.LastStayAwakeCall()
	if call == nil {
		t.Fatal("expected stayAwake call")
	}
	if call.Caption != "Transcoding video: Encoding in background" {
		t.Fatalf("unexpected notification caption: %q", call.Caption)
	}
	if resp.ExpiresAt != now.Add(2*time.Minute) {
		t.Fatalf("unexpected expiry: %s", resp.ExpiresAt)
	}

	now = now.Add(30 * time.Second)
	expiresAt, err := broker.Renew(resp.LockID, 3*time.Minute)
	if err != nil {
		t.Fatalf("Renew returned error: %v", err)
	}
	if expiresAt != now.Add(3*time.Minute) {
		t.Fatalf("unexpected renewed expiry: %s", expiresAt)
	}

	if err := broker.Release(resp.LockID); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	waitForRelease(t, call.HandleServer.ReleaseCh)
}

func TestBrokerSweepExpiredReleasesHandle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 15, 4, 5, 0, time.UTC)
	apiServer := &testcapnp.SandstormAPIServer{}
	broker := NewBroker(Options{
		Client: testcapnp.NewClient(&testcapnp.BridgeServer{APIServer: apiServer}),
		Now:    func() time.Time { return now },
	})

	resp, err := broker.Acquire(context.Background(), AcquireRequest{
		SessionID: "session-1",
		TTL:       30 * time.Second,
		Title:     "Transcoding video",
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	call := apiServer.LastStayAwakeCall()
	now = now.Add(31 * time.Second)
	broker.sweepExpired()
	waitForRelease(t, call.HandleServer.ReleaseCh)

	if _, err := broker.Renew(resp.LockID, time.Minute); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("expected lock not found after sweep, got %v", err)
	}
}

func TestBrokerCancelDropsLock(t *testing.T) {
	t.Parallel()

	apiServer := &testcapnp.SandstormAPIServer{}
	broker := NewBroker(Options{
		Client: testcapnp.NewClient(&testcapnp.BridgeServer{APIServer: apiServer}),
	})

	resp, err := broker.Acquire(context.Background(), AcquireRequest{
		SessionID: "session-1",
		TTL:       time.Minute,
		Title:     "Transcoding video",
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	call := apiServer.LastStayAwakeCall()
	broker.cancel(resp.LockID)
	waitForRelease(t, call.HandleServer.ReleaseCh)
	if err := broker.Release(resp.LockID); !errors.Is(err, ErrLockNotFound) {
		t.Fatalf("expected lock to be removed after cancel, got %v", err)
	}
}

func TestClientProtocol(t *testing.T) {
	t.Parallel()

	apiServer := &testcapnp.SandstormAPIServer{}
	broker := NewBroker(Options{
		Client: testcapnp.NewClient(&testcapnp.BridgeServer{APIServer: apiServer}),
	})

	resp, err := doProtocolRequest(t, broker, protocolRequest{
		Op:        "acquire",
		SessionID: "session-1",
		TTL:       time.Minute.String(),
		Title:     "Transcoding video",
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	if !resp.OK || resp.LockID == "" {
		t.Fatalf("unexpected acquire response: %+v", resp)
	}
	lockID := resp.LockID

	resp, err = doProtocolRequest(t, broker, protocolRequest{
		Op:     "renew",
		LockID: lockID,
		TTL:    time.Minute.String(),
	})
	if err != nil {
		t.Fatalf("Renew returned error: %v", err)
	}
	if !resp.OK || resp.ExpiresAt.IsZero() {
		t.Fatalf("unexpected renew response: %+v", resp)
	}

	resp, err = doProtocolRequest(t, broker, protocolRequest{
		Op:     "release",
		LockID: lockID,
	})
	if err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if !resp.OK {
		t.Fatalf("unexpected release response: %+v", resp)
	}
}

func waitForRelease(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handle release")
	}
}

func doProtocolRequest(t *testing.T, broker *Broker, req protocolRequest) (protocolResponse, error) {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		broker.handleConn(context.Background(), serverConn)
	}()

	if err := json.NewEncoder(clientConn).Encode(req); err != nil {
		return protocolResponse{}, err
	}

	var resp protocolResponse
	if err := json.NewDecoder(clientConn).Decode(&resp); err != nil {
		return protocolResponse{}, err
	}
	<-done
	return resp, nil
}
