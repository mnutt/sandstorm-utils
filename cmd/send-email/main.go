package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mnutt/sandstorm-utils/internal/cliutil"
	"github.com/mnutt/sandstorm-utils/internal/sendemail"
)

const commandName = "send-email"
const commandPurpose = "Send an email through the current Sandstorm session."
const commandSynopsis = "[--timeout 10s] [--json-input FILE|-] [--from ADDR] [--from-name NAME] [--to ADDR] [--cc ADDR] [--bcc ADDR] [--reply-to ADDR] [--reply-to-name NAME] [--subject TEXT] [--text TEXT|--text-file FILE|-] [--html TEXT|--html-file FILE|-] <sessionId>"

var commandExamples = []string{
	"Send a plain-text email with direct flags.\nCommand: send-email --to user@example.com --subject \"Hello\" --text \"Hi there\" <sessionId>\nArguments: --to is the recipient address, --subject sets the subject line, --text sets the plain-text body, and <sessionId> is the Sandstorm session ID for the current request.\nEffect: asks Sandstorm to send the message through the current grain's email capability.\nReturns: no output on success.",
	"Send an email from a JSON message definition when the message has more structure.\nCommand: send-email --json-input message.json <sessionId>\nArguments: --json-input points to a JSON file describing the message and <sessionId> is the Sandstorm session ID for the current request.\nEffect: asks Sandstorm to send the structured message from the JSON payload.\nReturns: no output on success.",
}

type addressListFlag []sendemail.Address

func (f *addressListFlag) add(value string) error {
	*f = append(*f, sendemail.Address{Address: value})
	return nil
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
	jsonInput := fs.String("json-input", "", "read email payload from JSON file or - for stdin")
	from := fs.String("from", "", "sender email address; defaults to get-user-address")
	fromName := fs.String("from-name", "", "sender display name")
	replyTo := fs.String("reply-to", "", "reply-to email address")
	replyToName := fs.String("reply-to-name", "", "reply-to display name")
	subject := fs.String("subject", "", "email subject")
	text := fs.String("text", "", "plain-text body")
	textFile := fs.String("text-file", "", "read plain-text body from file or - for stdin")
	html := fs.String("html", "", "HTML body")
	htmlFile := fs.String("html-file", "", "read HTML body from file or - for stdin")

	var to addressListFlag
	var cc addressListFlag
	var bcc addressListFlag
	fs.Func("to", "recipient email address; repeat for multiple recipients", to.add)
	fs.Func("cc", "cc recipient email address; repeat for multiple recipients", cc.add)
	fs.Func("bcc", "bcc recipient email address; repeat for multiple recipients", bcc.add)

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

	opts := sendemail.Options{
		To:      append([]sendemail.Address(nil), to...),
		Cc:      append([]sendemail.Address(nil), cc...),
		Bcc:     append([]sendemail.Address(nil), bcc...),
		Subject: *subject,
		Text:    *text,
		HTML:    *html,
	}
	if *from != "" || *fromName != "" {
		opts.From = &sendemail.Address{Address: *from, Name: *fromName}
	}
	if *replyTo != "" || *replyToName != "" {
		opts.ReplyTo = &sendemail.Address{Address: *replyTo, Name: *replyToName}
	}

	if *textFile != "" {
		value, err := readInput(*textFile)
		if err != nil {
			return fmt.Errorf("read text body: %w", err)
		}
		opts.Text = value
	}
	if *htmlFile != "" {
		value, err := readInput(*htmlFile)
		if err != nil {
			return fmt.Errorf("read html body: %w", err)
		}
		opts.HTML = value
	}

	if *jsonInput != "" {
		payload, err := sendemail.ReadJSONInput(*jsonInput)
		if err != nil {
			return err
		}
		opts = opts.MergeJSONInput(payload)
		if visited["from"] || visited["from-name"] {
			if opts.From == nil {
				opts.From = &sendemail.Address{}
			}
			if visited["from"] {
				opts.From.Address = *from
			}
			if visited["from-name"] {
				opts.From.Name = *fromName
			}
		}
		if visited["to"] {
			opts.To = append([]sendemail.Address(nil), to...)
		}
		if visited["cc"] {
			opts.Cc = append([]sendemail.Address(nil), cc...)
		}
		if visited["bcc"] {
			opts.Bcc = append([]sendemail.Address(nil), bcc...)
		}
		if visited["reply-to"] || visited["reply-to-name"] {
			if opts.ReplyTo == nil {
				opts.ReplyTo = &sendemail.Address{}
			}
			if visited["reply-to"] {
				opts.ReplyTo.Address = *replyTo
			}
			if visited["reply-to-name"] {
				opts.ReplyTo.Name = *replyToName
			}
		}
		if visited["subject"] {
			opts.Subject = *subject
		}
		if visited["text"] {
			opts.Text = *text
		}
		if visited["text-file"] {
			value, err := readInput(*textFile)
			if err != nil {
				return fmt.Errorf("read text body: %w", err)
			}
			opts.Text = value
		}
		if visited["html"] {
			opts.HTML = *html
		}
		if visited["html-file"] {
			value, err := readInput(*htmlFile)
			if err != nil {
				return fmt.Errorf("read html body: %w", err)
			}
			opts.HTML = value
		}
	}

	return sendemail.Run(ctx, fs.Arg(0), opts)
}

func readInput(path string) (string, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}
