package sendemail

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestRunWithClient(t *testing.T) {
	t.Parallel()

	sessionServer := &testcapnp.SessionServer{
		UserAddress: "owner@example.com",
		UserName:    "Owner Name",
	}
	client := testcapnp.NewClient(&testcapnp.BridgeServer{SessionServer: sessionServer})

	err := RunWithClient(context.Background(), client, "session-1", Options{
		To:      []Address{{Address: "user@example.com", Name: "User Name"}},
		Subject: "Hello",
		Text:    "Plain text body",
		HTML:    "<p>HTML body</p>",
	})
	if err != nil {
		t.Fatalf("RunWithClient returned error: %v", err)
	}

	if len(sessionServer.SentEmails) != 1 {
		t.Fatalf("expected 1 send call, got %d", len(sessionServer.SentEmails))
	}

	call := sessionServer.SentEmails[0]
	if call.From.Address != "owner@example.com" || call.From.Name != "Owner Name" {
		t.Fatalf("unexpected from address: %+v", call.From)
	}
	if len(call.To) != 1 || call.To[0].Address != "user@example.com" || call.To[0].Name != "User Name" {
		t.Fatalf("unexpected to addresses: %+v", call.To)
	}
	if call.Subject != "Hello" || call.Text != "Plain text body" || call.HTML != "<p>HTML body</p>" {
		t.Fatalf("unexpected email payload: %+v", call)
	}
}

func TestReadJSONInputAndMerge(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "send-email-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	const payload = `{
  "from": {"address":"from@example.com","name":"Sender"},
  "to": ["to@example.com"],
  "cc": [{"address":"cc@example.com","name":"CC User"}],
  "replyTo": "reply@example.com",
  "subject": "From JSON",
  "text": "JSON body"
}`
	if _, err := f.WriteString(payload); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	parsed, err := ReadJSONInput(f.Name())
	if err != nil {
		t.Fatalf("ReadJSONInput returned error: %v", err)
	}

	opts := Options{
		Subject: "Flag subject",
		HTML:    "<p>HTML</p>",
	}.MergeJSONInput(parsed)

	if opts.From == nil || opts.From.Address != "from@example.com" || opts.From.Name != "Sender" {
		t.Fatalf("unexpected merged from: %+v", opts.From)
	}
	if len(opts.To) != 1 || opts.To[0].Address != "to@example.com" {
		t.Fatalf("unexpected merged to: %+v", opts.To)
	}
	if len(opts.Cc) != 1 || opts.Cc[0].Name != "CC User" {
		t.Fatalf("unexpected merged cc: %+v", opts.Cc)
	}
	if opts.ReplyTo == nil || opts.ReplyTo.Address != "reply@example.com" {
		t.Fatalf("unexpected merged replyTo: %+v", opts.ReplyTo)
	}
	if opts.Subject != "From JSON" || opts.Text != "JSON body" || opts.HTML != "<p>HTML</p>" {
		t.Fatalf("unexpected merged message fields: %+v", opts)
	}
}

func TestReadJSONInputRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "send-email-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(`{"bogus":true}`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	_, err = ReadJSONInput(f.Name())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWithClientRejectsMissingRecipients(t *testing.T) {
	t.Parallel()

	sessionServer := &testcapnp.SessionServer{
		UserAddress: "owner@example.com",
		UserName:    "Owner Name",
	}
	client := testcapnp.NewClient(&testcapnp.BridgeServer{SessionServer: sessionServer})

	err := RunWithClient(context.Background(), client, "session-1", Options{
		Subject: "Hello",
		Text:    "Body",
	})
	if err == nil || !strings.Contains(err.Error(), "at least one recipient is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
