package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/sessioninspect"
)

const commandName = "get-session-offer"
const commandPurpose = "Inspect the Powerbox offer attached to a Sandstorm session."
const commandSynopsis = "[--timeout 10s] <sessionId>"

var commandExamples = []string{
	"Inspect the Powerbox offer attached to the current session.\nCommand: get-session-offer <sessionId>\nArguments: <sessionId> is the Sandstorm session ID for the current request.\nReturns: JSON describing the attached Powerbox offer and any decoded tag payloads.",
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

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName, commandSynopsis)
	}

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	data, err := sessioninspect.GetSessionOffer(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	return cliutil.WriteOutput(data, true)
}
