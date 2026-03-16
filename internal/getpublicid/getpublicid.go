package getpublicid

import (
	"context"
	"encoding/json"
	"fmt"

	capnp "capnproto.org/go/capnp/v3"

	hacksession "github.com/mnutt/sandstorm-utils/internal/generated/hacksession"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type Result struct {
	PublicID   string `json:"publicId"`
	Hostname   string `json:"hostname"`
	AutoURL    string `json:"autoUrl"`
	IsDemoUser bool   `json:"isDemoUser"`
}

func Format(result Result, asJSON bool) ([]byte, error) {
	if asJSON {
		return json.MarshalIndent(result, "", "  ")
	}
	return []byte(fmt.Sprintf("%s\n%s\n%s\n%t\n", result.PublicID, result.Hostname, result.AutoURL, result.IsDemoUser)), nil
}

func Fetch(ctx context.Context, sessionID string) (_ Result, err error) {
	return FetchWithClient(ctx, sandstorm.NewClient(), sessionID)
}

func FetchWithClient(ctx context.Context, client *sandstorm.Client, sessionID string) (result Result, err error) {
	err = client.WithBridge(ctx, func(bridge sandstormhttpbridge.SandstormHttpBridge) error {
		session, release, err := sandstorm.ResolveHackSession(ctx, bridge, sessionID)
		if err != nil {
			return err
		}
		defer release()
		defer capnp.Client(session).Release()

		result, err = getPublicID(ctx, session)
		return err
	})
	return result, err
}

func getPublicID(ctx context.Context, session hacksession.HackSessionContext) (Result, error) {
	result, release := session.GetPublicId(ctx, nil)
	defer release()

	payload, err := result.Struct()
	if err != nil {
		return Result{}, fmt.Errorf("getPublicId RPC failed: %w", err)
	}

	publicID, err := payload.PublicId()
	if err != nil {
		return Result{}, fmt.Errorf("read publicId: %w", err)
	}
	hostname, err := payload.Hostname()
	if err != nil {
		return Result{}, fmt.Errorf("read hostname: %w", err)
	}
	autoURL, err := payload.AutoUrl()
	if err != nil {
		return Result{}, fmt.Errorf("read autoUrl: %w", err)
	}

	return Result{
		PublicID:   publicID,
		Hostname:   hostname,
		AutoURL:    autoURL,
		IsDemoUser: payload.IsDemoUser(),
	}, nil
}
