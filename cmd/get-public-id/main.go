package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/getpublicid"
)

const commandName = "get-public-id"
const commandPurpose = "Fetch the current grain's public ID metadata."
const commandSynopsis = "[--timeout 10s] [--json] <sessionId>"

var commandExamples = []string{
	"get-public-id <sessionId>",
	"get-public-id --json <sessionId>",
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

	result, err := getpublicid.Fetch(ctx, fs.Arg(0))
	if err != nil {
		return err
	}

	output, err := getpublicid.Format(result, *asJSON)
	if err != nil {
		return err
	}
	return cliutil.WriteOutput(output, *asJSON)
}
