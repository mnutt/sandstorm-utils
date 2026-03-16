package closesession

import (
	"context"
	"fmt"

	capnp "capnproto.org/go/capnp/v3"

	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

func Run(ctx context.Context, sessionID string) error {
	return RunWithClient(ctx, sandstorm.NewClient(), sessionID)
}

func RunWithClient(ctx context.Context, client *sandstorm.Client, sessionID string) error {
	return client.WithBridge(ctx, func(bridge sandstormhttpbridge.SandstormHttpBridge) error {
		session, release, err := sandstorm.ResolveSessionContext(ctx, bridge, sessionID)
		if err != nil {
			return err
		}
		defer release()
		defer capnp.Client(session).Release()

		return closeSession(ctx, session)
	})
}

func closeSession(ctx context.Context, session grain.SessionContext) error {
	result, release := session.Close(ctx, nil)
	defer release()

	if _, err := result.Struct(); err != nil {
		return fmt.Errorf("close RPC failed: %w", err)
	}

	return nil
}
