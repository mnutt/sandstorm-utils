package openview

import (
	"context"
	"fmt"

	capnp "capnproto.org/go/capnp/v3"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type Options struct {
	Path   string
	NewTab bool
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

		return openView(ctx, session, opts)
	})
}

func openView(ctx context.Context, session grain.SessionContext, opts Options) error {
	opts.Path = cliutil.NormalizeGrainPath(opts.Path)

	result, release := session.OpenView(ctx, func(p grain.SessionContext_openView_Params) error {
		if opts.Path != "" {
			if err := p.SetPath(opts.Path); err != nil {
				return err
			}
		}
		p.SetNewTab(opts.NewTab)
		return nil
	})
	defer release()

	if _, err := result.Struct(); err != nil {
		return fmt.Errorf("openView RPC failed: %w", err)
	}

	return nil
}
