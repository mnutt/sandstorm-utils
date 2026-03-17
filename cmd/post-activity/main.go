package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/postactivity"
)

const commandName = "post-activity"
const commandPurpose = "Post a structured activity event to the current Sandstorm session."
const commandSynopsis = "[--timeout 10s] [--json-input FILE|-] [--path PATH] [--type N] [--thread-path PATH] [--thread-title TITLE] [--caption TEXT] <sessionId>"

var commandExamples = []string{
	"Post a simple activity event using direct flags.\nCommand: post-activity --path /issues/1#comment-2 --type 3 --thread-path /issues/1 --thread-title \"Issue 1\" --caption \"New comment\" <sessionId>\nArguments: --path is the grain-relative activity path, --type is the app-defined event type index, --thread-path and --thread-title identify the thread, --caption sets the notification caption, and <sessionId> is the Sandstorm session ID for the current request.\nEffect: creates an activity or notification entry visible through Sandstorm.\nReturns: no output on success.",
	"Post a richer activity event from JSON when the payload is too large or structured for flags.\nCommand: post-activity --json-input event.json <sessionId>\nArguments: --json-input points to a JSON file containing the activity payload and <sessionId> is the Sandstorm session ID for the current request.\nEffect: creates an activity or notification entry using the structured JSON payload.\nReturns: no output on success.",
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
	jsonInput := fs.String("json-input", "", "read activity payload from JSON file or - for stdin")
	path := fs.String("path", "", "activity path inside the grain")
	eventType := fs.Uint("type", 0, "activity type index")
	threadPath := fs.String("thread-path", "", "thread path")
	threadTitle := fs.String("thread-title", "", "thread title")
	caption := fs.String("caption", "", "notification caption")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cliutil.UsageError(commandName, commandSynopsis)
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})

	ctx, cancel := cliutil.ContextWithTimeout(parent, *timeout)
	defer cancel()

	opts := postactivity.Options{
		Path: *path,
		Type: uint16(*eventType),
	}
	if *threadPath != "" || *threadTitle != "" {
		opts.Thread = &postactivity.ThreadOptions{
			Path: *threadPath,
		}
		if *threadTitle != "" {
			opts.Thread.Title = &postactivity.LocalizedTextInput{DefaultText: *threadTitle}
		}
	}
	if *caption != "" {
		opts.Notification = &postactivity.NotificationOptions{
			Caption: &postactivity.LocalizedTextInput{DefaultText: *caption},
		}
	}
	if *jsonInput != "" {
		payload, err := postactivity.ReadJSONInput(*jsonInput)
		if err != nil {
			return err
		}
		opts = opts.MergeJSONInput(payload)
		if visited["path"] {
			opts.Path = *path
		}
		if visited["type"] {
			opts.Type = uint16(*eventType)
		}
		if visited["thread-path"] {
			if opts.Thread == nil {
				opts.Thread = &postactivity.ThreadOptions{}
			}
			opts.Thread.Path = *threadPath
		}
		if visited["thread-title"] {
			if opts.Thread == nil {
				opts.Thread = &postactivity.ThreadOptions{}
			}
			opts.Thread.Title = &postactivity.LocalizedTextInput{DefaultText: *threadTitle}
		}
		if visited["caption"] {
			if opts.Notification == nil {
				opts.Notification = &postactivity.NotificationOptions{}
			}
			opts.Notification.Caption = &postactivity.LocalizedTextInput{DefaultText: *caption}
		}
	}

	return postactivity.Run(ctx, fs.Arg(0), opts)
}
