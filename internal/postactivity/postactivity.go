package postactivity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	capnp "capnproto.org/go/capnp/v3"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	activity "github.com/mnutt/sandstorm-utils/internal/generated/activity"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	identity "github.com/mnutt/sandstorm-utils/internal/generated/identity"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	util "github.com/mnutt/sandstorm-utils/internal/generated/util"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type Options struct {
	Path         string
	Type         uint16
	Thread       *ThreadOptions
	Notification *NotificationOptions
	Users        []UserOptions
}

type ThreadOptions struct {
	Path  string
	Title *LocalizedTextInput
}

type NotificationOptions struct {
	Caption *LocalizedTextInput
}

type UserOptions struct {
	IdentityID string `json:"identityId"`
	Mentioned  bool   `json:"mentioned,omitempty"`
	Subscribed bool   `json:"subscribed,omitempty"`
	CanView    bool   `json:"canView,omitempty"`
}

type LocalizedTextInput struct {
	DefaultText   string                  `json:"defaultText"`
	Localizations []LocalizationTextInput `json:"localizations,omitempty"`
}

func (l *LocalizedTextInput) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("localized text must not be empty")
	}
	if data[0] == '"' {
		return json.Unmarshal(data, &l.DefaultText)
	}

	type rawLocalizedText LocalizedTextInput
	var raw rawLocalizedText
	if err := decodeStrictJSON(bytes.NewReader(data), &raw); err != nil {
		return err
	}
	*l = LocalizedTextInput(raw)
	return nil
}

type LocalizationTextInput struct {
	Locale string `json:"locale"`
	Text   string `json:"text"`
}

type JSONInput struct {
	Path         *string                `json:"path,omitempty"`
	Type         *uint16                `json:"type,omitempty"`
	Thread       *ThreadJSONInput       `json:"thread,omitempty"`
	Notification *NotificationJSONInput `json:"notification,omitempty"`
	Users        []UserOptions          `json:"users,omitempty"`
}

type ThreadJSONInput struct {
	Path  *string             `json:"path,omitempty"`
	Title *LocalizedTextInput `json:"title,omitempty"`
}

type NotificationJSONInput struct {
	Caption *LocalizedTextInput `json:"caption,omitempty"`
}

func ReadJSONInput(path string) (JSONInput, error) {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return JSONInput{}, fmt.Errorf("open json input: %w", err)
		}
		defer f.Close()
		r = f
	}

	var payload JSONInput
	if err := decodeStrictJSON(r, &payload); err != nil {
		return JSONInput{}, fmt.Errorf("decode json input: %w", err)
	}
	if err := validateJSONInput(payload); err != nil {
		return JSONInput{}, err
	}

	return payload, nil
}

func decodeStrictJSON(r io.Reader, dst any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return fmt.Errorf("extra content after top-level JSON value")
		}
		return err
	}
	return nil
}

func (o Options) MergeJSONInput(payload JSONInput) Options {
	merged := o

	if payload.Path != nil {
		merged.Path = *payload.Path
	}
	if payload.Type != nil {
		merged.Type = *payload.Type
	}
	if payload.Thread != nil {
		merged.Thread = &ThreadOptions{}
		if payload.Thread.Path != nil {
			merged.Thread.Path = *payload.Thread.Path
		}
		if payload.Thread.Title != nil {
			merged.Thread.Title = payload.Thread.Title
		}
	}
	if payload.Notification != nil {
		merged.Notification = &NotificationOptions{}
		if payload.Notification.Caption != nil {
			merged.Notification.Caption = payload.Notification.Caption
		}
	}
	if len(payload.Users) > 0 {
		merged.Users = append([]UserOptions(nil), payload.Users...)
	}

	return merged
}

func Run(ctx context.Context, sessionID string, opts Options) error {
	return RunWithClient(ctx, sandstorm.NewClient(), sessionID, opts)
}

func RunWithClient(ctx context.Context, client *sandstorm.Client, sessionID string, opts Options) error {
	return client.WithBridge(ctx, func(bridge sandstormhttpbridge.SandstormHttpBridge) error {
		session, release, err := sandstorm.ResolveSessionContext(ctx, bridge, sessionID)
		if err != nil {
			return err
		}
		defer release()
		defer capnp.Client(session).Release()

		return postActivity(ctx, bridge, session, opts)
	})
}

func postActivity(ctx context.Context, bridge sandstormhttpbridge.SandstormHttpBridge, session grain.SessionContext, opts Options) error {
	opts.Path = cliutil.NormalizeGrainPath(opts.Path)
	if opts.Thread != nil {
		opts.Thread.Path = cliutil.NormalizeGrainPath(opts.Thread.Path)
	}

	resolvedUsers, releaseUsers, err := resolveUsers(ctx, bridge, opts.Users)
	if err != nil {
		return err
	}
	defer releaseUsers()

	result, release := session.Activity(ctx, func(p grain.SessionContext_activity_Params) error {
		event, err := buildActivityEvent(p, opts, resolvedUsers)
		if err != nil {
			return err
		}
		return p.SetEvent(event)
	})
	defer release()

	if _, err := result.Struct(); err != nil {
		return fmt.Errorf("activity RPC failed: %w", err)
	}

	return nil
}

type resolvedUser struct {
	Identity identity.Identity
	UserOptions
}

func buildActivityEvent(params grain.SessionContext_activity_Params, opts Options, users []resolvedUser) (activity.ActivityEvent, error) {
	event, err := params.NewEvent()
	if err != nil {
		return activity.ActivityEvent{}, err
	}

	if err := event.SetPath(opts.Path); err != nil {
		return activity.ActivityEvent{}, err
	}
	event.SetType(opts.Type)

	if opts.Thread != nil {
		thread, err := event.NewThread()
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		if err := thread.SetPath(opts.Thread.Path); err != nil {
			return activity.ActivityEvent{}, err
		}
		if opts.Thread.Title != nil {
			if err := setLocalizedText(thread.NewTitle, opts.Thread.Title); err != nil {
				return activity.ActivityEvent{}, err
			}
		}
	}

	if opts.Notification != nil && opts.Notification.Caption != nil {
		notification, err := event.NewNotification()
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		if err := setLocalizedText(notification.NewCaption, opts.Notification.Caption); err != nil {
			return activity.ActivityEvent{}, err
		}
	}

	if len(users) > 0 {
		list, err := event.NewUsers(int32(len(users)))
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		for i, user := range users {
			item := list.At(i)
			if err := item.SetIdentity(user.Identity); err != nil {
				return activity.ActivityEvent{}, err
			}
			item.SetMentioned(user.Mentioned)
			item.SetSubscribed(user.Subscribed)
			item.SetCanView(user.CanView)
		}
	}

	return event, nil
}

func resolveUsers(ctx context.Context, bridge sandstormhttpbridge.SandstormHttpBridge, users []UserOptions) ([]resolvedUser, func(), error) {
	if len(users) == 0 {
		return nil, func() {}, nil
	}

	resolved := make([]resolvedUser, 0, len(users))
	release := func() {
		for _, user := range resolved {
			if user.Identity.IsValid() {
				user.Identity.Release()
			}
		}
	}

	for _, user := range users {
		result, resultRelease := bridge.GetSavedIdentity(ctx, func(p sandstormhttpbridge.SandstormHttpBridge_getSavedIdentity_Params) error {
			return p.SetIdentityId(user.IdentityID)
		})

		if _, err := result.Struct(); err != nil {
			resultRelease()
			release()
			return nil, func() {}, fmt.Errorf("getSavedIdentity RPC failed for %q: %w", user.IdentityID, err)
		}

		idCap := result.Identity()
		resultRelease()
		if !idCap.IsValid() {
			release()
			return nil, func() {}, fmt.Errorf("getSavedIdentity returned a null identity capability for %q", user.IdentityID)
		}

		resolved = append(resolved, resolvedUser{
			Identity:    idCap,
			UserOptions: user,
		})
	}

	return resolved, release, nil
}

func validateJSONInput(payload JSONInput) error {
	if payload.Thread != nil && payload.Thread.Title != nil {
		if err := validateLocalizedText("thread.title", *payload.Thread.Title); err != nil {
			return err
		}
	}
	if payload.Notification != nil && payload.Notification.Caption != nil {
		if err := validateLocalizedText("notification.caption", *payload.Notification.Caption); err != nil {
			return err
		}
	}
	for i, user := range payload.Users {
		if strings.TrimSpace(user.IdentityID) == "" {
			return fmt.Errorf("invalid users[%d]: identityId is required", i)
		}
		if !user.Mentioned && !user.Subscribed && !user.CanView {
			return fmt.Errorf("invalid users[%d]: at least one of mentioned, subscribed, or canView must be true", i)
		}
	}
	return nil
}

func validateLocalizedText(path string, text LocalizedTextInput) error {
	for i, localization := range text.Localizations {
		if strings.TrimSpace(localization.Locale) == "" {
			return fmt.Errorf("invalid %s.localizations[%d]: locale is required", path, i)
		}
	}
	return nil
}

func setLocalizedText(newText func() (util.LocalizedText, error), input *LocalizedTextInput) error {
	text, err := newText()
	if err != nil {
		return err
	}
	if input == nil {
		return nil
	}
	if err := text.SetDefaultText(input.DefaultText); err != nil {
		return err
	}
	if len(input.Localizations) == 0 {
		return nil
	}

	localizations, err := text.NewLocalizations(int32(len(input.Localizations)))
	if err != nil {
		return err
	}
	for i, localization := range input.Localizations {
		item := localizations.At(i)
		if err := item.SetLocale(localization.Locale); err != nil {
			return err
		}
		if err := item.SetText(localization.Text); err != nil {
			return err
		}
	}

	return nil
}
