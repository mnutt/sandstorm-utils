package sendemail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	capnp "capnproto.org/go/capnp/v3"

	email "github.com/mnutt/sandstorm-utils/internal/generated/email"
	hacksession "github.com/mnutt/sandstorm-utils/internal/generated/hacksession"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type Address struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

func (a *Address) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("address must not be empty")
	}
	if data[0] == '"' {
		return json.Unmarshal(data, &a.Address)
	}

	type rawAddress Address
	var raw rawAddress
	if err := decodeStrictJSON(bytes.NewReader(data), &raw); err != nil {
		return err
	}
	*a = Address(raw)
	return nil
}

type Options struct {
	From    *Address
	To      []Address
	Cc      []Address
	Bcc     []Address
	ReplyTo *Address
	Subject string
	Text    string
	HTML    string
}

type JSONInput struct {
	From    *Address  `json:"from,omitempty"`
	To      []Address `json:"to,omitempty"`
	Cc      []Address `json:"cc,omitempty"`
	Bcc     []Address `json:"bcc,omitempty"`
	ReplyTo *Address  `json:"replyTo,omitempty"`
	Subject *string   `json:"subject,omitempty"`
	Text    *string   `json:"text,omitempty"`
	HTML    *string   `json:"html,omitempty"`
}

func ReadJSONInput(path string) (JSONInput, error) {
	var r io.Reader
	if path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return JSONInput{}, fmt.Errorf("open json input: %w", err)
		}
		defer f.Close()
		r = f
	}

	var payload JSONInput
	if err := decodeStrictJSON(r, &payload); err != nil {
		return JSONInput{}, fmt.Errorf("decode json input: %w", err)
	}
	if err := validateJSONInput(payload); err != nil {
		return JSONInput{}, err
	}

	return payload, nil
}

func decodeStrictJSON(r io.Reader, dst any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return fmt.Errorf("extra content after top-level JSON value")
		}
		return err
	}
	return nil
}

func (o Options) MergeJSONInput(payload JSONInput) Options {
	merged := o

	if payload.From != nil {
		merged.From = payload.From
	}
	if len(payload.To) > 0 {
		merged.To = append([]Address(nil), payload.To...)
	}
	if len(payload.Cc) > 0 {
		merged.Cc = append([]Address(nil), payload.Cc...)
	}
	if len(payload.Bcc) > 0 {
		merged.Bcc = append([]Address(nil), payload.Bcc...)
	}
	if payload.ReplyTo != nil {
		merged.ReplyTo = payload.ReplyTo
	}
	if payload.Subject != nil {
		merged.Subject = *payload.Subject
	}
	if payload.Text != nil {
		merged.Text = *payload.Text
	}
	if payload.HTML != nil {
		merged.HTML = *payload.HTML
	}

	return merged
}

func Run(ctx context.Context, sessionID string, opts Options) error {
	return RunWithClient(ctx, sandstorm.NewClient(), sessionID, opts)
}

func RunWithClient(ctx context.Context, client *sandstorm.Client, sessionID string, opts Options) error {
	return client.WithBridge(ctx, func(bridge sandstormhttpbridge.SandstormHttpBridge) error {
		session, release, err := sandstorm.ResolveHackSession(ctx, bridge, sessionID)
		if err != nil {
			return err
		}
		defer release()
		defer capnp.Client(session).Release()

		return sendEmail(ctx, session, opts)
	})
}

func sendEmail(ctx context.Context, session hacksession.HackSessionContext, opts Options) error {
	resolved, err := finalizeOptions(ctx, session, opts)
	if err != nil {
		return err
	}

	result, release := session.Send(ctx, func(p email.EmailSendPort_send_Params) error {
		message, err := p.NewEmail()
		if err != nil {
			return err
		}

		message.SetDate(time.Now().UnixNano())
		if err := setAddress(message.NewFrom, *resolved.From); err != nil {
			return err
		}
		if err := setAddressList(message.NewTo, resolved.To); err != nil {
			return err
		}
		if err := setAddressList(message.NewCc, resolved.Cc); err != nil {
			return err
		}
		if err := setAddressList(message.NewBcc, resolved.Bcc); err != nil {
			return err
		}
		if resolved.ReplyTo != nil {
			if err := setAddress(message.NewReplyTo, *resolved.ReplyTo); err != nil {
				return err
			}
		}
		if err := message.SetSubject(resolved.Subject); err != nil {
			return err
		}
		if err := message.SetText(resolved.Text); err != nil {
			return err
		}
		if err := message.SetHtml(resolved.HTML); err != nil {
			return err
		}
		return nil
	})
	defer release()

	if _, err := result.Struct(); err != nil {
		return fmt.Errorf("send RPC failed: %w", err)
	}

	return nil
}

func finalizeOptions(ctx context.Context, session hacksession.HackSessionContext, opts Options) (Options, error) {
	resolved := opts
	if resolved.From == nil {
		from, err := defaultFrom(ctx, session)
		if err != nil {
			return Options{}, err
		}
		resolved.From = &from
	}
	if err := validateOptions(resolved); err != nil {
		return Options{}, err
	}
	return resolved, nil
}

func defaultFrom(ctx context.Context, session hacksession.HackSessionContext) (Address, error) {
	result, release := session.GetUserAddress(ctx, nil)
	defer release()

	addr, err := result.Struct()
	if err != nil {
		return Address{}, fmt.Errorf("getUserAddress RPC failed: %w", err)
	}

	emailAddress, err := addr.Address()
	if err != nil {
		return Address{}, fmt.Errorf("read address: %w", err)
	}
	name, err := addr.Name()
	if err != nil {
		return Address{}, fmt.Errorf("read name: %w", err)
	}

	return Address{Address: emailAddress, Name: name}, nil
}

func validateJSONInput(payload JSONInput) error {
	if payload.From != nil {
		if err := validateAddress("from", *payload.From); err != nil {
			return err
		}
	}
	if payload.ReplyTo != nil {
		if err := validateAddress("replyTo", *payload.ReplyTo); err != nil {
			return err
		}
	}
	if err := validateAddressList("to", payload.To); err != nil {
		return err
	}
	if err := validateAddressList("cc", payload.Cc); err != nil {
		return err
	}
	if err := validateAddressList("bcc", payload.Bcc); err != nil {
		return err
	}
	return nil
}

func validateOptions(opts Options) error {
	if opts.From == nil {
		return fmt.Errorf("from address is required")
	}
	if err := validateAddress("from", *opts.From); err != nil {
		return err
	}
	if opts.ReplyTo != nil {
		if err := validateAddress("replyTo", *opts.ReplyTo); err != nil {
			return err
		}
	}
	if err := validateAddressList("to", opts.To); err != nil {
		return err
	}
	if err := validateAddressList("cc", opts.Cc); err != nil {
		return err
	}
	if err := validateAddressList("bcc", opts.Bcc); err != nil {
		return err
	}
	if len(opts.To) == 0 && len(opts.Cc) == 0 && len(opts.Bcc) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	if strings.TrimSpace(opts.Text) == "" && strings.TrimSpace(opts.HTML) == "" {
		return fmt.Errorf("at least one of text or html is required")
	}
	return nil
}

func validateAddressList(path string, list []Address) error {
	for i, addr := range list {
		if err := validateAddress(fmt.Sprintf("%s[%d]", path, i), addr); err != nil {
			return err
		}
	}
	return nil
}

func validateAddress(path string, addr Address) error {
	if strings.TrimSpace(addr.Address) == "" {
		return fmt.Errorf("invalid %s: address is required", path)
	}
	return nil
}

func setAddress(newAddr func() (email.EmailAddress, error), input Address) error {
	addr, err := newAddr()
	if err != nil {
		return err
	}
	if err := addr.SetAddress(input.Address); err != nil {
		return err
	}
	if err := addr.SetName(input.Name); err != nil {
		return err
	}
	return nil
}

func setAddressList(newList func(int32) (email.EmailAddress_List, error), list []Address) error {
	values, err := newList(int32(len(list)))
	if err != nil {
		return err
	}
	for i, input := range list {
		item := values.At(i)
		if err := item.SetAddress(input.Address); err != nil {
			return err
		}
		if err := item.SetName(input.Name); err != nil {
			return err
		}
	}
	return nil
}
