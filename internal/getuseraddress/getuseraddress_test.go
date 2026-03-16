package getuseraddress

import (
	"context"
	"strings"
	"testing"

	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestFetchWithClient(t *testing.T) {
	t.Parallel()

	client := testcapnp.NewClient(&testcapnp.BridgeServer{
		SessionServer: &testcapnp.SessionServer{
			UserAddress: "owner@example.com",
			UserName:    "Owner Name",
		},
	})

	result, err := FetchWithClient(context.Background(), client, "session-1")
	if err != nil {
		t.Fatalf("FetchWithClient returned error: %v", err)
	}

	if result.Address != "owner@example.com" || result.Name != "Owner Name" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	result := Result{
		Address: "owner@example.com",
		Name:    "Owner Name",
	}

	text, err := Format(result, false)
	if err != nil {
		t.Fatalf("Format text returned error: %v", err)
	}
	if string(text) != "owner@example.com\nOwner Name\n" {
		t.Fatalf("unexpected text output: %q", string(text))
	}

	jsonData, err := Format(result, true)
	if err != nil {
		t.Fatalf("Format JSON returned error: %v", err)
	}
	if !strings.Contains(string(jsonData), "\"address\": \"owner@example.com\"") {
		t.Fatalf("unexpected JSON output: %s", string(jsonData))
	}
}
