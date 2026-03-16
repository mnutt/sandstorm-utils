package cliutil

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func NewFlagSet(name, purpose, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		if purpose != "" {
			fmt.Fprintln(fs.Output(), purpose)
			fmt.Fprintln(fs.Output())
		}
		fmt.Fprintf(fs.Output(), "Usage:\n  %s %s\n\n", name, synopsis)
		fmt.Fprintln(fs.Output(), "Flags:")
		fs.PrintDefaults()
	}
	return fs
}

func UsageError(commandName, synopsis string) error {
	return fmt.Errorf("usage: %s %s", commandName, synopsis)
}

func ContextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return context.WithTimeout(parent, timeout)
}

func WriteOutput(output []byte, appendNewline bool) error {
	if appendNewline {
		output = append(output, '\n')
	}
	_, err := os.Stdout.Write(output)
	return err
}

func NormalizeGrainPath(path string) string {
	return strings.TrimPrefix(path, "/")
}
