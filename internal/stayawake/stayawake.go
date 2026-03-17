package stayawake

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"

	activity "github.com/mnutt/sandstorm-utils/internal/generated/activity"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	util "github.com/mnutt/sandstorm-utils/internal/generated/util"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type Options struct {
	Client *sandstorm.Client
}

type AcquireRequest struct {
	Title   string
	Caption string
}

type HeldLock struct {
	mu      sync.Mutex
	entry   *lockEntry
	release sync.Once
}

type lockEntry struct {
	handle       util.Handle
	notification activity.OngoingNotification
	conn         net.Conn
	rpcConn      *rpc.Conn
}

type notificationServer struct {
	lock *HeldLock
}

var ErrTitleRequired = errors.New("title is required")

func AcquireHeldLock(ctx context.Context, opts Options, req AcquireRequest) (*HeldLock, error) {
	if strings.TrimSpace(req.Title) == "" {
		return nil, ErrTitleRequired
	}

	client := opts.Client
	if client == nil {
		client = sandstorm.NewClient()
	}

	lock := &HeldLock{}
	notification := activity.OngoingNotification_ServerToClient(&notificationServer{lock: lock})
	caption := joinNotificationText(req.Title, req.Caption)

	handle, conn, rpcConn, err := acquireWakeLock(ctx, client, caption, notification)
	if err != nil {
		notification.Release()
		return nil, err
	}

	lock.entry = &lockEntry{
		handle:       handle,
		notification: notification,
		conn:         conn,
		rpcConn:      rpcConn,
	}
	return lock, nil
}

func (l *HeldLock) Release() error {
	if l == nil {
		return nil
	}

	l.release.Do(func() {
		l.mu.Lock()
		entry := l.entry
		l.entry = nil
		l.mu.Unlock()
		releaseEntry(entry)
	})
	return nil
}

func acquireWakeLock(ctx context.Context, client *sandstorm.Client, caption string, notification activity.OngoingNotification) (util.Handle, net.Conn, *rpc.Conn, error) {
	conn, err := client.DialContext(ctx)
	if err != nil {
		return util.Handle{}, nil, nil, err
	}

	rpcConn := rpc.NewConn(rpc.NewStreamTransport(conn), nil)
	bridge := sandstormhttpbridge.SandstormHttpBridge(rpcConn.Bootstrap(ctx))
	defer capnp.Client(bridge).Release()

	api, release, err := sandstorm.ResolveSandstormAPI(ctx, bridge)
	if err != nil {
		_ = rpcConn.Close()
		_ = conn.Close()
		return util.Handle{}, nil, nil, err
	}
	defer release()
	defer capnp.Client(api).Release()

	result, callRelease := api.StayAwake(ctx, func(p grain.SandstormApi_stayAwake_Params) error {
		displayInfo, err := p.NewDisplayInfo()
		if err != nil {
			return err
		}
		localized, err := displayInfo.NewCaption()
		if err != nil {
			return err
		}
		if err := localized.SetDefaultText(caption); err != nil {
			return err
		}
		return p.SetNotification(notification)
	})
	defer callRelease()

	results, err := result.Struct()
	if err != nil {
		_ = rpcConn.Close()
		_ = conn.Close()
		return util.Handle{}, nil, nil, fmt.Errorf("stayAwake RPC failed: %w", err)
	}

	handle := results.Handle()
	if !handle.IsValid() {
		_ = rpcConn.Close()
		_ = conn.Close()
		return util.Handle{}, nil, nil, errors.New("stayAwake returned a null handle")
	}

	// Retain the capability after releasing the RPC answer so the wake lock stays alive.
	handle = handle.AddRef()
	return handle, conn, rpcConn, nil
}

func (n *notificationServer) Cancel(context.Context, activity.OngoingNotification_cancel) error {
	return n.lock.Release()
}

func joinNotificationText(title, caption string) string {
	title = strings.TrimSpace(title)
	caption = strings.TrimSpace(caption)
	switch {
	case title == "":
		return caption
	case caption == "":
		return title
	default:
		return title + ": " + caption
	}
}

func releaseEntry(entry *lockEntry) {
	if entry == nil {
		return
	}
	if entry.handle.IsValid() {
		entry.handle.Release()
	}
	if entry.notification.IsValid() {
		entry.notification.Release()
	}
	if entry.rpcConn != nil {
		_ = entry.rpcConn.Close()
	}
	if entry.conn != nil {
		_ = entry.conn.Close()
	}
}
