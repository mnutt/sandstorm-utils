package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/entergrain"
)

const commandName = "enter-grain"
const commandPurpose = "Enter a Sandstorm grain's Linux namespaces and launch a shell there."
const commandSynopsis = "<pid>"

var commandExamples = []string{
	"Attach to a running grain process by PID.\nCommand: enter-grain <pid>\nArguments: <pid> is the process ID of a grain process already running inside Sandstorm.\nEffect: joins that process's user, IPC, UTS, network, PID, and mount namespaces, switches to its current working directory, and launches /bin/bash with the target environment.\nReturns: an interactive shell session or a non-zero exit status on failure.",
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

func run(_ context.Context, args []string) error {
	fs := cliutil.NewFlagSet(commandName, commandPurpose, commandSynopsis)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName, commandSynopsis)
	}

	return entergrain.Run(fs.Arg(0), entergrain.Options{})
}
