package getuseraddress

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
	Address string `json:"address"`
	Name    string `json:"name"`
}

func Format(result Result, asJSON bool) ([]byte, error) {
	if asJSON {
		return json.MarshalIndent(result, "", "  ")
	}
	return []byte(fmt.Sprintf("%s\n%s\n", result.Address, result.Name)), nil
}

func Fetch(ctx context.Context, sessionID string) (Result, error) {
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

		result, err = fetchUserAddress(ctx, session)
		return err
	})

	return result, err
}

func fetchUserAddress(ctx context.Context, session hacksession.HackSessionContext) (Result, error) {
	result, release := session.GetUserAddress(ctx, nil)
	defer release()

	address, err := result.Struct()
	if err != nil {
		return Result{}, fmt.Errorf("getUserAddress RPC failed: %w", err)
	}

	emailAddress, err := address.Address()
	if err != nil {
		return Result{}, fmt.Errorf("read address: %w", err)
	}

	name, err := address.Name()
	if err != nil {
		return Result{}, fmt.Errorf("read name: %w", err)
	}

	return Result{
		Address: emailAddress,
		Name:    name,
	}, nil
}
