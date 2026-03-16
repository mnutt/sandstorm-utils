package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/getuseraddress"
)

const commandName = "get-user-address"
const commandPurpose = "Fetch the authenticated user's email address and display name."
const commandSynopsis = "[--timeout 10s] [--json] <sessionId>"

var commandExamples = []string{
	"get-user-address <sessionId>",
	"get-user-address --json <sessionId>",
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
	timeout := fs.Duration("timeout", 10*time.Second, "RPC timeout")
	asJSON := fs.Bool("json", false, "emit JSON instead of newline-delimited text")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName, commandSynopsis)
	}

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	result, err := getuseraddress.Fetch(ctx, fs.Arg(0))
	if err != nil {
		return err
	}

	output, err := getuseraddress.Format(result, *asJSON)
	if err != nil {
		return err
	}
	return cliutil.WriteOutput(output, *asJSON)
}
