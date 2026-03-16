package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
	"github.com/mnutt/sandstorm-utils/internal/stayawake"
)

const commandName = "stay-awake"

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "acquire":
		return runAcquire(parent, args[1:])
	case "renew":
		return runRenew(parent, args[1:])
	case "release":
		return runRelease(parent, args[1:])
	case "serve":
		return runServe(parent, args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return usageError()
	}
}

func runAcquire(parent context.Context, args []string) error {
	fs := cliutil.NewFlagSet(commandName+" acquire", "Acquire a Sandstorm wake lock lease.", "[--timeout 10s] [--socket PATH] [--api-path PATH] [--ttl 10m] --title TEXT [--caption TEXT] <sessionId>")
	timeout := fs.Duration("timeout", 10*time.Second, "command timeout")
	socketPath := fs.String("socket", stayawake.DefaultSocketPath(), "broker Unix socket path")
	apiPath := fs.String("api-path", sandstorm.DefaultAPIPath, "Sandstorm API Unix socket path")
	ttl := fs.Duration("ttl", stayawake.DefaultTTL, "lease time-to-live")
	title := fs.String("title", "", "notification title")
	caption := fs.String("caption", "", "notification caption")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName+" acquire", "[--timeout 10s] [--socket PATH] [--api-path PATH] [--ttl 10m] --title TEXT [--caption TEXT] <sessionId>")
	}

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	client, err := connectClient(ctx, *socketPath, *apiPath)
	if err != nil {
		return err
	}

	resp, err := client.Acquire(ctx, stayawake.AcquireRequest{
		SessionID: fs.Arg(0),
		TTL:       *ttl,
		Title:     *title,
		Caption:   *caption,
	})
	if err != nil {
		return err
	}

	output, err := json.Marshal(struct {
		LockID    string    `json:"lockId"`
		ExpiresAt time.Time `json:"expiresAt"`
	}{
		LockID:    resp.LockID,
		ExpiresAt: resp.ExpiresAt,
	})
	if err != nil {
		return err
	}
	return cliutil.WriteOutput(output, true)
}

func runRenew(parent context.Context, args []string) error {
	fs := cliutil.NewFlagSet(commandName+" renew", "Renew a wake lock lease.", "[--timeout 10s] [--socket PATH] [--api-path PATH] [--ttl 10m] <lockId>")
	timeout := fs.Duration("timeout", 10*time.Second, "command timeout")
	socketPath := fs.String("socket", stayawake.DefaultSocketPath(), "broker Unix socket path")
	apiPath := fs.String("api-path", sandstorm.DefaultAPIPath, "Sandstorm API Unix socket path")
	ttl := fs.Duration("ttl", stayawake.DefaultTTL, "lease time-to-live")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName+" renew", "[--timeout 10s] [--socket PATH] [--api-path PATH] [--ttl 10m] <lockId>")
	}

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	client, err := connectClient(ctx, *socketPath, *apiPath)
	if err != nil {
		return err
	}

	expiresAt, err := client.Renew(ctx, fs.Arg(0), *ttl)
	if err != nil {
		return err
	}

	output, err := json.Marshal(struct {
		ExpiresAt time.Time `json:"expiresAt"`
	}{ExpiresAt: expiresAt})
	if err != nil {
		return err
	}
	return cliutil.WriteOutput(output, true)
}

func runRelease(parent context.Context, args []string) error {
	fs := cliutil.NewFlagSet(commandName+" release", "Release a wake lock lease.", "[--timeout 10s] [--socket PATH] [--api-path PATH] <lockId>")
	timeout := fs.Duration("timeout", 10*time.Second, "command timeout")
	socketPath := fs.String("socket", stayawake.DefaultSocketPath(), "broker Unix socket path")
	apiPath := fs.String("api-path", sandstorm.DefaultAPIPath, "Sandstorm API Unix socket path")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName+" release", "[--timeout 10s] [--socket PATH] [--api-path PATH] <lockId>")
	}

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	client, err := connectClient(ctx, *socketPath, *apiPath)
	if err != nil {
		return err
	}

	return client.Release(ctx, fs.Arg(0))
}

func runServe(parent context.Context, args []string) error {
	fs := flag.NewFlagSet(commandName+" serve", flag.ContinueOnError)
	fs.SetOutput(ioDiscard{})
	socketPath := fs.String("socket", stayawake.DefaultSocketPath(), "")
	apiPath := fs.String("api-path", sandstorm.DefaultAPIPath, "")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := sandstorm.NewClient()
	client.APIPath = *apiPath
	return stayawake.NewBroker(stayawake.Options{Client: client}).Serve(parent, *socketPath)
}

func connectClient(ctx context.Context, socketPath, apiPath string) (*stayawake.Client, error) {
	client := stayawake.NewClient(socketPath)
	if err := ensureBroker(ctx, socketPath, apiPath); err != nil {
		return nil, err
	}
	return client, nil
}

func ensureBroker(ctx context.Context, socketPath, apiPath string) error {
	client := stayawake.NewClient(socketPath)
	if err := client.Release(ctx, "__probe__"); err == nil || !isDialError(err) {
		return nil
	}

	if err := startBroker(socketPath, apiPath); err != nil {
		return err
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := client.Release(ctx, "__probe__")
		if err == nil || !isDialError(err) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("broker did not start at %s", socketPath)
}

func startBroker(socketPath, apiPath string) error {
	exe, err := resolveSelfPath()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "serve", "--socket", socketPath, "--api-path", apiPath)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()

	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func resolveSelfPath() (string, error) {
	if len(os.Args) == 0 || strings.TrimSpace(os.Args[0]) == "" {
		return "", errors.New("resolve executable path: argv[0] is empty")
	}

	exePath := os.Args[0]
	if !filepath.IsAbs(exePath) {
		absPath, err := filepath.Abs(exePath)
		if err != nil {
			return "", fmt.Errorf("resolve executable path: %w", err)
		}
		exePath = absPath
	}

	return exePath, nil
}

func isDialError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage:\n  %s <command> [flags]\n\nCommands:\n  acquire  acquire a wake lock lease\n  renew    renew a wake lock lease\n  release  release a wake lock lease\n", commandName)
}

func usageError() error {
	return fmt.Errorf("usage: %s <acquire|renew|release> [flags]", commandName)
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
