package sandstorm

import (
	"context"
	"fmt"
	"net"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"

	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	hacksession "github.com/mnutt/sandstorm-utils/internal/generated/hacksession"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
)

const DefaultAPIPath = "/tmp/sandstorm-api"

type DialFunc func(context.Context) (net.Conn, error)

type Client struct {
	APIPath string
	Dial    DialFunc
}

func NewClient() *Client {
	return &Client{APIPath: DefaultAPIPath}
}

func (c *Client) DialContext(ctx context.Context) (net.Conn, error) {
	return c.dial(ctx)
}

func (c *Client) WithBridge(ctx context.Context, fn func(sandstormhttpbridge.SandstormHttpBridge) error) (err error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	rpcConn := rpc.NewConn(rpc.NewStreamTransport(conn), nil)
	defer func() {
		closeErr := rpcConn.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	bridge := sandstormhttpbridge.SandstormHttpBridge(rpcConn.Bootstrap(ctx))
	defer capnp.Client(bridge).Release()

	return fn(bridge)
}

func ResolveSandstormAPI(ctx context.Context, bridge sandstormhttpbridge.SandstormHttpBridge) (grain.SandstormApi, capnp.ReleaseFunc, error) {
	result, release := bridge.GetSandstormApi(ctx, nil)

	results, err := result.Struct()
	if err != nil {
		release()
		return grain.SandstormApi{}, nil, fmt.Errorf("getSandstormApi RPC failed: %w", err)
	}

	api := results.Api()
	if !capnp.Client(api).IsValid() {
		release()
		return grain.SandstormApi{}, nil, fmt.Errorf("getSandstormApi returned a null API capability")
	}

	return api, release, nil
}

func ResolveSessionContext(ctx context.Context, bridge sandstormhttpbridge.SandstormHttpBridge, sessionID string) (grain.SessionContext, capnp.ReleaseFunc, error) {
	result, release := bridge.GetSessionContext(ctx, func(p sandstormhttpbridge.SandstormHttpBridge_getSessionContext_Params) error {
		return p.SetId(sessionID)
	})

	results, err := result.Struct()
	if err != nil {
		release()
		return grain.SessionContext{}, nil, fmt.Errorf("getSessionContext RPC failed: %w", err)
	}

	session := results.Context()
	if !capnp.Client(session).IsValid() {
		release()
		return grain.SessionContext{}, nil, fmt.Errorf("getSessionContext returned a null context capability")
	}

	return session, release, nil
}

func ResolveHackSession(ctx context.Context, bridge sandstormhttpbridge.SandstormHttpBridge, sessionID string) (hacksession.HackSessionContext, capnp.ReleaseFunc, error) {
	session, release, err := ResolveSessionContext(ctx, bridge, sessionID)
	if err != nil {
		return hacksession.HackSessionContext{}, nil, err
	}

	hackSession := hacksession.HackSessionContext(session)
	if !capnp.Client(hackSession).IsValid() {
		release()
		return hacksession.HackSessionContext{}, nil, fmt.Errorf("getSessionContext returned an invalid hack session capability")
	}

	return hackSession, release, nil
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c != nil && c.Dial != nil {
		return c.Dial(ctx)
	}

	apiPath := DefaultAPIPath
	if c != nil && c.APIPath != "" {
		apiPath = c.APIPath
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", apiPath)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", apiPath, err)
	}

	return conn, nil
}
