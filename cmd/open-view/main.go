package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/openview"
)

const commandName = "open-view"
const commandPurpose = "Ask Sandstorm to open a grain-relative path in the current or a new tab."
const commandSynopsis = "[--timeout 10s] [--path PATH] [--new-tab] <sessionId>"

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
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	path := fs.String("path", "", "path to open")
	newTab := fs.Bool("new-tab", false, "open in a new tab")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName, commandSynopsis)
	}

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	return openview.Run(ctx, fs.Arg(0), openview.Options{
		Path:   *path,
		NewTab: *newTab,
	})
}
