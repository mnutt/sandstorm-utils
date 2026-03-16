package postactivity

import (
	"context"
	"os"
	"strings"
	"testing"

	capnp "capnproto.org/go/capnp/v3"

	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	identity "github.com/mnutt/sandstorm-utils/internal/generated/identity"
	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestRunWithClient(t *testing.T) {
	t.Parallel()

	sessionServer := &testcapnp.SessionServer{}
	client := testcapnp.NewClient(&testcapnp.BridgeServer{SessionServer: sessionServer})

	err := RunWithClient(context.Background(), client, "session-1", Options{
		Path: "/issues/1#comment-2",
		Type: 3,
		Thread: &ThreadOptions{
			Path:  "/issues/1",
			Title: &LocalizedTextInput{DefaultText: "Issue 1"},
		},
		Notification: &NotificationOptions{
			Caption: &LocalizedTextInput{DefaultText: "New comment"},
		},
	})
	if err != nil {
		t.Fatalf("RunWithClient returned error: %v", err)
	}

	if len(sessionServer.ActivityCalls) != 1 {
		t.Fatalf("expected 1 activity call, got %d", len(sessionServer.ActivityCalls))
	}

	call := sessionServer.ActivityCalls[0]
	if call.Path != "issues/1#comment-2" || call.Type != 3 || call.ThreadPath != "issues/1" || call.ThreadTitle != "Issue 1" || call.Caption != "New comment" {
		t.Fatalf("unexpected activity call: %+v", call)
	}
}

func TestReadJSONInputAndMerge(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "activity-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	const payload = `{
  "path": "/from-json",
  "type": 7,
  "thread": {
    "path": "/thread-json",
    "title": {
      "defaultText": "JSON title",
      "localizations": [
        {"locale": "fr", "text": "Titre JSON"}
      ]
    }
  },
  "notification": {
    "caption": {
      "defaultText": "JSON caption"
    }
  },
  "users": [
    {
      "identityId": "user-1",
      "mentioned": true
    }
  ]
}`
	if _, err := f.WriteString(payload); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	parsed, err := ReadJSONInput(f.Name())
	if err != nil {
		t.Fatalf("ReadJSONInput returned error: %v", err)
	}

	opts := Options{
		Path: "/flag-path",
		Thread: &ThreadOptions{
			Title: &LocalizedTextInput{DefaultText: "Flag title"},
		},
		Notification: &NotificationOptions{
			Caption: &LocalizedTextInput{DefaultText: "Flag caption"},
		},
	}.MergeJSONInput(parsed)

	if opts.Path != "/from-json" || opts.Type != 7 {
		t.Fatalf("unexpected merged top-level values: %+v", opts)
	}
	if opts.Thread == nil || opts.Thread.Path != "/thread-json" || opts.Thread.Title == nil || opts.Thread.Title.DefaultText != "JSON title" {
		t.Fatalf("unexpected merged thread values: %+v", opts.Thread)
	}
	if len(opts.Thread.Title.Localizations) != 1 || opts.Thread.Title.Localizations[0].Locale != "fr" {
		t.Fatalf("unexpected merged thread localizations: %+v", opts.Thread.Title.Localizations)
	}
	if opts.Notification == nil || opts.Notification.Caption == nil || opts.Notification.Caption.DefaultText != "JSON caption" {
		t.Fatalf("unexpected merged notification values: %+v", opts.Notification)
	}
	if len(opts.Users) != 1 || opts.Users[0].IdentityID != "user-1" || !opts.Users[0].Mentioned {
		t.Fatalf("unexpected merged users: %+v", opts.Users)
	}
}

func TestReadJSONInputSupportsStringLocalizedText(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "activity-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	const payload = `{
  "thread": {
    "title": "Short title"
  },
  "notification": {
    "caption": "Short caption"
  }
}`
	if _, err := f.WriteString(payload); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	parsed, err := ReadJSONInput(f.Name())
	if err != nil {
		t.Fatalf("ReadJSONInput returned error: %v", err)
	}
	if parsed.Thread == nil || parsed.Thread.Title == nil || parsed.Thread.Title.DefaultText != "Short title" {
		t.Fatalf("unexpected parsed thread title: %+v", parsed.Thread)
	}
	if parsed.Notification == nil || parsed.Notification.Caption == nil || parsed.Notification.Caption.DefaultText != "Short caption" {
		t.Fatalf("unexpected parsed caption: %+v", parsed.Notification)
	}
}

func TestReadJSONInputRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "activity-*.json")
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

func TestReadJSONInputRejectsUnknownLocalizedTextFields(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "activity-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(`{"thread":{"title":{"defaultText":"x","bogus":true}}}`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	_, err = ReadJSONInput(f.Name())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadJSONInputRejectsInvalidUsers(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "activity-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(`{"users":[{"identityId":"user-1"}]}`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	_, err = ReadJSONInput(f.Name())
	if err == nil || !strings.Contains(err.Error(), "at least one of mentioned, subscribed, or canView must be true") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadJSONInputRejectsInvalidLocalization(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "activity-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(`{"thread":{"title":{"defaultText":"x","localizations":[{"text":"Bonjour"}]}}}`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	_, err = ReadJSONInput(f.Name())
	if err == nil || !strings.Contains(err.Error(), "locale is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildActivityEventWithLocalizedJSON(t *testing.T) {
	t.Parallel()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	params, err := grain.NewRootSessionContext_activity_Params(seg)
	if err != nil {
		t.Fatalf("NewRootSessionContext_activity_Params: %v", err)
	}

	event, err := buildActivityEvent(params, Options{
		Path: "/issues/1",
		Type: 9,
		Thread: &ThreadOptions{
			Path: "/thread/1",
			Title: &LocalizedTextInput{
				DefaultText: "Issue 1",
				Localizations: []LocalizationTextInput{
					{Locale: "fr", Text: "Probleme 1"},
				},
			},
		},
		Notification: &NotificationOptions{
			Caption: &LocalizedTextInput{
				DefaultText: "Alerte",
				Localizations: []LocalizationTextInput{
					{Locale: "es", Text: "Alerta"},
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("buildActivityEvent returned error: %v", err)
	}

	thread, err := event.Thread()
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	title, err := thread.Title()
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	defaultTitle, err := title.DefaultText()
	if err != nil {
		t.Fatalf("DefaultText: %v", err)
	}
	if defaultTitle != "Issue 1" {
		t.Fatalf("unexpected title default text: %q", defaultTitle)
	}
	titleLocalizations, err := title.Localizations()
	if err != nil {
		t.Fatalf("Title.Localizations: %v", err)
	}
	if titleLocalizations.Len() != 1 {
		t.Fatalf("expected 1 title localization, got %d", titleLocalizations.Len())
	}
	if locale, _ := titleLocalizations.At(0).Locale(); locale != "fr" {
		t.Fatalf("unexpected title locale: %q", locale)
	}

	notification, err := event.Notification()
	if err != nil {
		t.Fatalf("Notification: %v", err)
	}
	caption, err := notification.Caption()
	if err != nil {
		t.Fatalf("Caption: %v", err)
	}
	captionLocalizations, err := caption.Localizations()
	if err != nil {
		t.Fatalf("Caption.Localizations: %v", err)
	}
	if captionLocalizations.Len() != 1 {
		t.Fatalf("expected 1 caption localization, got %d", captionLocalizations.Len())
	}
	if text, _ := captionLocalizations.At(0).Text(); text != "Alerta" {
		t.Fatalf("unexpected caption localization text: %q", text)
	}
}

func TestBuildActivityEventUsers(t *testing.T) {
	t.Parallel()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	params, err := grain.NewRootSessionContext_activity_Params(seg)
	if err != nil {
		t.Fatalf("NewRootSessionContext_activity_Params: %v", err)
	}

	idCap := testcapnp.NewIdentityCapability()
	t.Cleanup(idCap.Release)

	event, err := buildActivityEvent(params, Options{
		Path: "/issues/2",
	}, []resolvedUser{
		{
			Identity: identity.Identity(idCap),
			UserOptions: UserOptions{
				IdentityID: "user-1",
				Mentioned:  true,
				CanView:    true,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildActivityEvent returned error: %v", err)
	}

	users, err := event.Users()
	if err != nil {
		t.Fatalf("Users: %v", err)
	}
	if users.Len() != 1 {
		t.Fatalf("expected 1 user, got %d", users.Len())
	}
	if !users.At(0).HasIdentity() || !users.At(0).Mentioned() || !users.At(0).CanView() || users.At(0).Subscribed() {
		t.Fatalf("unexpected event user flags")
	}
}
