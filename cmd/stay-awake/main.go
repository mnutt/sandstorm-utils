package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
	"github.com/mnutt/sandstorm-utils/internal/stayawake"
)

const commandName = "stay-awake"
const commandPurpose = "Keep a Sandstorm wake lock active for the lifetime of the helper process."
const commandSynopsis = "[--timeout 10s] [--api-path PATH] [--for DURATION] --title TEXT [--caption TEXT]"

var commandExamples = []string{
	"Run stay-awake as a child helper while background work is in progress.\nCommand: stay-awake --title \"Transcoding video\" --caption \"Encoding in the background\"\nArguments: --title and --caption control the notification text. Spawn this helper as a subprocess and keep its stdin open while the background task is active.\nLock lifetime: the wake lock stays active until the stay-awake process exits, its stdin is closed, or it receives SIGTERM, SIGHUP, or SIGINT.\nReturns: no output on success.",
	"Hold a wake lock for a bounded amount of time.\nCommand: stay-awake --for 30s --title \"Transcoding video\" --caption \"Encoding in the background\"\nArguments: --for sets the maximum time to hold the lock and --title and --caption control the notification text.\nLock lifetime: the wake lock is released when 30s elapse or earlier if the stay-awake process exits, its stdin is closed, or it receives SIGTERM, SIGHUP, or SIGINT.\nReturns: no output on success.",
}

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
	fs := cliutil.NewFlagSet(commandName, commandPurpose, commandSynopsis)
	timeout := fs.Duration("timeout", 10*time.Second, "command timeout while acquiring the wake lock")
	apiPath := fs.String("api-path", sandstorm.DefaultAPIPath, "Sandstorm API Unix socket path")
	holdFor := fs.Duration("for", 0, "maximum time to hold the wake lock before exiting")
	title := fs.String("title", "", "notification title")
	caption := fs.String("caption", "", "notification caption")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return cliutil.UsageError(commandName, commandSynopsis)
	}
	if *holdFor < 0 {
		return errors.New("--for must be non-negative")
	}

	acquireCtx, cancelAcquire := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancelAcquire()

	client := sandstorm.NewClient()
	client.APIPath = *apiPath

	lock, err := stayawake.AcquireHeldLock(acquireCtx, stayawake.Options{Client: client}, stayawake.AcquireRequest{
		Title:     *title,
		Caption:   *caption,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	return waitForRelease(parent, *holdFor)
}

func waitForRelease(parent context.Context, holdFor time.Duration) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	stdinDone := watchStdin(os.Stdin)
	var timer <-chan time.Time
	if holdFor > 0 {
		t := time.NewTimer(holdFor)
		defer t.Stop()
		timer = t.C
	}

	select {
	case <-ctx.Done():
		return nil
	case <-stdinDone:
		return nil
	case <-timer:
		return nil
	}
}

func watchStdin(r *os.File) <-chan struct{} {
	done := make(chan struct{})
	if shouldIgnoreStdin(r) {
		return done
	}

	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, r)
	}()

	return done
}

func shouldIgnoreStdin(r *os.File) bool {
	if r == nil {
		return true
	}
	info, err := r.Stat()
	if err != nil {
		return true
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
