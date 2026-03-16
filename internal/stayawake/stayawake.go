package stayawake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"

	activity "github.com/mnutt/sandstorm-utils/internal/generated/activity"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	util "github.com/mnutt/sandstorm-utils/internal/generated/util"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

const (
	DefaultTTL          = 10 * time.Minute
	MinimumTTL          = 30 * time.Second
	DefaultReapInterval = time.Second
)

type Client struct {
	SocketPath string
}

type Broker struct {
	client       *sandstorm.Client
	now          func() time.Time
	reapInterval time.Duration

	mu    sync.Mutex
	locks map[string]*lockEntry
}

type lockEntry struct {
	id           string
	sessionID    string
	expiresAt    time.Time
	caption      string
	handle       util.Handle
	notification activity.OngoingNotification
	conn         net.Conn
	rpcConn      *rpc.Conn
}

type Options struct {
	Client       *sandstorm.Client
	Now          func() time.Time
	ReapInterval time.Duration
}

type AcquireRequest struct {
	SessionID string
	TTL       time.Duration
	Title     string
	Caption   string
}

type AcquireResponse struct {
	LockID    string    `json:"lockId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type protocolRequest struct {
	Op        string `json:"op"`
	SessionID string `json:"sessionId,omitempty"`
	LockID    string `json:"lockId,omitempty"`
	TTL       string `json:"ttl,omitempty"`
	Title     string `json:"title,omitempty"`
	Caption   string `json:"caption,omitempty"`
}

type protocolResponse struct {
	OK        bool      `json:"ok"`
	LockID    string    `json:"lockId,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	Error     string    `json:"error,omitempty"`
}

var (
	ErrLockNotFound  = errors.New("lock not found")
	ErrLockExpired   = errors.New("lock expired")
	ErrTitleRequired = errors.New("title is required")
)

func NewClient(socketPath string) *Client {
	return &Client{SocketPath: socketPath}
}

func DefaultSocketPath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("sandstorm-stay-awake-%d.sock", os.Getuid()))
}

func NewBroker(opts Options) *Broker {
	client := opts.Client
	if client == nil {
		client = sandstorm.NewClient()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	reapInterval := opts.ReapInterval
	if reapInterval <= 0 {
		reapInterval = DefaultReapInterval
	}
	return &Broker{
		client:       client,
		now:          now,
		reapInterval: reapInterval,
		locks:        map[string]*lockEntry{},
	}
}

func (c *Client) Acquire(ctx context.Context, req AcquireRequest) (AcquireResponse, error) {
	resp, err := c.do(ctx, protocolRequest{
		Op:        "acquire",
		SessionID: req.SessionID,
		TTL:       req.TTL.String(),
		Title:     req.Title,
		Caption:   req.Caption,
	})
	if err != nil {
		return AcquireResponse{}, err
	}
	return AcquireResponse{LockID: resp.LockID, ExpiresAt: resp.ExpiresAt}, nil
}

func (c *Client) Renew(ctx context.Context, lockID string, ttl time.Duration) (time.Time, error) {
	resp, err := c.do(ctx, protocolRequest{
		Op:     "renew",
		LockID: lockID,
		TTL:    ttl.String(),
	})
	if err != nil {
		return time.Time{}, err
	}
	return resp.ExpiresAt, nil
}

func (c *Client) Release(ctx context.Context, lockID string) error {
	_, err := c.do(ctx, protocolRequest{
		Op:     "release",
		LockID: lockID,
	})
	return err
}

func (c *Client) do(ctx context.Context, req protocolRequest) (protocolResponse, error) {
	socketPath := DefaultSocketPath()
	if c != nil && c.SocketPath != "" {
		socketPath = c.SocketPath
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return protocolResponse{}, err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return protocolResponse{}, err
	}

	var resp protocolResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return protocolResponse{}, err
	}
	if !resp.OK {
		return protocolResponse{}, errors.New(resp.Error)
	}
	return resp, nil
}

func (b *Broker) Serve(ctx context.Context, socketPath string) error {
	listener, err := listenUnix(socketPath)
	if err != nil {
		return err
	}
	defer func() {
		listener.Close()
		_ = os.Remove(socketPath)
	}()

	var wg sync.WaitGroup
	stopReaper := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.runReaper(stopReaper)
	}()

	go func() {
		<-ctx.Done()
		close(stopReaper)
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				b.shutdown()
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				continue
			}
			wg.Wait()
			b.shutdown()
			return err
		}

		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer conn.Close()
			b.handleConn(ctx, conn)
		}(conn)
	}
}

func (b *Broker) Acquire(ctx context.Context, req AcquireRequest) (AcquireResponse, error) {
	ttl, err := normalizeTTL(req.TTL)
	if err != nil {
		return AcquireResponse{}, err
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return AcquireResponse{}, errors.New("session ID is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return AcquireResponse{}, ErrTitleRequired
	}

	lockID, err := randomID()
	if err != nil {
		return AcquireResponse{}, err
	}

	caption := joinNotificationText(req.Title, req.Caption)
	notificationServer := &notificationServer{broker: b, lockID: lockID}
	notification := activity.OngoingNotification_ServerToClient(notificationServer)

	handle, conn, rpcConn, err := b.acquireWakeLock(ctx, caption, notification)
	if err != nil {
		notification.Release()
		return AcquireResponse{}, err
	}

	expiresAt := b.now().Add(ttl)
	b.mu.Lock()
	b.locks[lockID] = &lockEntry{
		id:           lockID,
		sessionID:    req.SessionID,
		expiresAt:    expiresAt,
		caption:      caption,
		handle:       handle,
		notification: notification,
		conn:         conn,
		rpcConn:      rpcConn,
	}
	b.mu.Unlock()

	return AcquireResponse{
		LockID:    lockID,
		ExpiresAt: expiresAt.UTC(),
	}, nil
}

func (b *Broker) Renew(lockID string, ttl time.Duration) (time.Time, error) {
	ttl, err := normalizeTTL(ttl)
	if err != nil {
		return time.Time{}, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.locks[lockID]
	if !ok {
		return time.Time{}, ErrLockNotFound
	}
	if !entry.expiresAt.After(b.now()) {
		delete(b.locks, lockID)
		go releaseEntry(entry)
		return time.Time{}, ErrLockExpired
	}

	entry.expiresAt = b.now().Add(ttl)
	return entry.expiresAt.UTC(), nil
}

func (b *Broker) Release(lockID string) error {
	entry, ok := b.removeLock(lockID)
	if !ok {
		return ErrLockNotFound
	}
	releaseEntry(entry)
	return nil
}

func (b *Broker) acquireWakeLock(ctx context.Context, caption string, notification activity.OngoingNotification) (util.Handle, net.Conn, *rpc.Conn, error) {
	conn, err := b.client.DialContext(ctx)
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

	return handle, conn, rpcConn, nil
}

func (b *Broker) handleConn(ctx context.Context, conn net.Conn) {
	var req protocolRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		writeResponse(conn, protocolResponse{Error: err.Error()})
		return
	}

	resp := protocolResponse{OK: true}
	switch req.Op {
	case "acquire":
		ttl, err := parseTTL(req.TTL)
		if err != nil {
			resp = errorResponse(err)
			break
		}
		result, err := b.Acquire(ctx, AcquireRequest{
			SessionID: req.SessionID,
			TTL:       ttl,
			Title:     req.Title,
			Caption:   req.Caption,
		})
		if err != nil {
			resp = errorResponse(err)
			break
		}
		resp.LockID = result.LockID
		resp.ExpiresAt = result.ExpiresAt
	case "renew":
		ttl, err := parseTTL(req.TTL)
		if err != nil {
			resp = errorResponse(err)
			break
		}
		expiresAt, err := b.Renew(req.LockID, ttl)
		if err != nil {
			resp = errorResponse(err)
			break
		}
		resp.ExpiresAt = expiresAt
	case "release":
		if err := b.Release(req.LockID); err != nil {
			resp = errorResponse(err)
		}
	default:
		resp = errorResponse(fmt.Errorf("unknown op %q", req.Op))
	}

	writeResponse(conn, resp)
}

func (b *Broker) runReaper(stop <-chan struct{}) {
	ticker := time.NewTicker(b.reapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.sweepExpired()
		case <-stop:
			return
		}
	}
}

func (b *Broker) sweepExpired() {
	now := b.now()

	var expired []*lockEntry
	b.mu.Lock()
	for id, entry := range b.locks {
		if !entry.expiresAt.After(now) {
			expired = append(expired, entry)
			delete(b.locks, id)
		}
	}
	b.mu.Unlock()

	for _, entry := range expired {
		releaseEntry(entry)
	}
}

func (b *Broker) cancel(lockID string) {
	entry, ok := b.removeLock(lockID)
	if !ok {
		return
	}
	releaseEntry(entry)
}

func (b *Broker) removeLock(lockID string) (*lockEntry, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.locks[lockID]
	if !ok {
		return nil, false
	}
	delete(b.locks, lockID)
	return entry, true
}

func (b *Broker) shutdown() {
	b.mu.Lock()
	locks := make([]*lockEntry, 0, len(b.locks))
	for id, entry := range b.locks {
		locks = append(locks, entry)
		delete(b.locks, id)
	}
	b.mu.Unlock()

	for _, entry := range locks {
		releaseEntry(entry)
	}
}

type notificationServer struct {
	broker *Broker
	lockID string
}

func (n *notificationServer) Cancel(context.Context, activity.OngoingNotification_cancel) error {
	n.broker.cancel(n.lockID)
	return nil
}

func normalizeTTL(ttl time.Duration) (time.Duration, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if ttl < MinimumTTL {
		return 0, fmt.Errorf("ttl must be at least %s", MinimumTTL)
	}
	return ttl, nil
}

func parseTTL(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return DefaultTTL, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse ttl: %w", err)
	}
	return ttl, nil
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

func randomID() (string, error) {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate lock ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
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

func errorResponse(err error) protocolResponse {
	return protocolResponse{OK: false, Error: err.Error()}
}

func writeResponse(w io.Writer, resp protocolResponse) {
	_ = json.NewEncoder(w).Encode(resp)
}

func listenUnix(socketPath string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}

	listener, err := net.Listen("unix", socketPath)
	if err == nil {
		return listener, nil
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return nil, err
	}
	if !errors.Is(opErr.Err, syscall.EADDRINUSE) {
		return nil, err
	}

	conn, dialErr := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
	if dialErr == nil {
		conn.Close()
		return nil, err
	}
	_ = os.Remove(socketPath)
	return net.Listen("unix", socketPath)
}
