package getpublicid

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
			PublicIDResult: testcapnp.PublicIDResult{
				PublicID:   "abc123",
				Hostname:   "example.com",
				AutoURL:    "https://abc123.example.com",
				IsDemoUser: true,
			},
		},
	})

	result, err := FetchWithClient(context.Background(), client, "session-1")
	if err != nil {
		t.Fatalf("FetchWithClient returned error: %v", err)
	}

	if result.PublicID != "abc123" || result.Hostname != "example.com" || result.AutoURL != "https://abc123.example.com" || !result.IsDemoUser {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	result := Result{
		PublicID:   "abc123",
		Hostname:   "example.com",
		AutoURL:    "https://abc123.example.com",
		IsDemoUser: true,
	}

	text, err := Format(result, false)
	if err != nil {
		t.Fatalf("Format text returned error: %v", err)
	}
	if string(text) != "abc123\nexample.com\nhttps://abc123.example.com\ntrue\n" {
		t.Fatalf("unexpected text output: %q", string(text))
	}

	jsonData, err := Format(result, true)
	if err != nil {
		t.Fatalf("Format JSON returned error: %v", err)
	}
	if !strings.Contains(string(jsonData), "\"publicId\": \"abc123\"") {
		t.Fatalf("unexpected JSON output: %s", string(jsonData))
	}
}
