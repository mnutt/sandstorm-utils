package sessioninspect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	capnp "capnproto.org/go/capnp/v3"

	apisession "github.com/mnutt/sandstorm-utils/internal/generated/apisession"
	email "github.com/mnutt/sandstorm-utils/internal/generated/email"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	identity "github.com/mnutt/sandstorm-utils/internal/generated/identity"
	ip "github.com/mnutt/sandstorm-utils/internal/generated/ip"
	powerbox "github.com/mnutt/sandstorm-utils/internal/generated/powerbox"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type DescriptorJSON struct {
	Quality string    `json:"quality"`
	TagIDs  []string  `json:"tagIds"`
	Tags    []TagJSON `json:"tags"`
	Text    string    `json:"text"`
}

type TagJSON struct {
	ID        string         `json:"id"`
	KnownType string         `json:"knownType,omitempty"`
	Value     map[string]any `json:"value,omitempty"`
	Text      string         `json:"text,omitempty"`
}

type RequestJSON struct {
	SessionType string           `json:"sessionType"`
	Descriptors []DescriptorJSON `json:"descriptors"`
}

type OfferJSON struct {
	SessionType string         `json:"sessionType"`
	Descriptor  DescriptorJSON `json:"descriptor"`
	Capability  CapabilityJSON `json:"capability"`
}

type CapabilityJSON struct {
	Present bool `json:"present"`
}

func GetSessionRequest(ctx context.Context, sessionID string) ([]byte, error) {
	return GetSessionRequestWithClient(ctx, sandstorm.NewClient(), sessionID)
}

func GetSessionRequestWithClient(ctx context.Context, client *sandstorm.Client, sessionID string) ([]byte, error) {
	var payload RequestJSON

	err := client.WithBridge(ctx, func(bridge sandstormhttpbridge.SandstormHttpBridge) error {
		result, release := bridge.GetSessionRequest(ctx, func(p sandstormhttpbridge.SandstormHttpBridge_getSessionRequest_Params) error {
			return p.SetId(sessionID)
		})
		defer release()

		results, err := result.Struct()
		if err != nil {
			return fmt.Errorf("getSessionRequest RPC failed: %w", err)
		}

		descriptors, err := results.RequestInfo()
		if err != nil {
			return fmt.Errorf("read requestInfo: %w", err)
		}

		rendered, err := renderDescriptors(descriptors)
		if err != nil {
			return err
		}

		payload = RequestJSON{
			SessionType: "request",
			Descriptors: rendered,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(payload, "", "  ")
}

func GetSessionOffer(ctx context.Context, sessionID string) ([]byte, error) {
	return GetSessionOfferWithClient(ctx, sandstorm.NewClient(), sessionID)
}

func GetSessionOfferWithClient(ctx context.Context, client *sandstorm.Client, sessionID string) ([]byte, error) {
	var payload OfferJSON

	err := client.WithBridge(ctx, func(bridge sandstormhttpbridge.SandstormHttpBridge) error {
		result, release := bridge.GetSessionOffer(ctx, func(p sandstormhttpbridge.SandstormHttpBridge_getSessionOffer_Params) error {
			return p.SetId(sessionID)
		})
		defer release()

		results, err := result.Struct()
		if err != nil {
			return fmt.Errorf("getSessionOffer RPC failed: %w", err)
		}

		descriptor, err := results.Descriptor()
		if err != nil {
			return fmt.Errorf("read descriptor: %w", err)
		}

		rendered, err := renderDescriptor(descriptor)
		if err != nil {
			return err
		}

		payload = OfferJSON{
			SessionType: "offer",
			Descriptor:  rendered,
			Capability: CapabilityJSON{
				Present: results.Offer().IsValid(),
			},
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(payload, "", "  ")
}

func renderDescriptors(list powerbox.PowerboxDescriptor_List) ([]DescriptorJSON, error) {
	rendered := make([]DescriptorJSON, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		desc, err := renderDescriptor(list.At(i))
		if err != nil {
			return nil, err
		}
		rendered = append(rendered, desc)
	}
	return rendered, nil
}

func renderDescriptor(desc powerbox.PowerboxDescriptor) (DescriptorJSON, error) {
	tags, err := desc.Tags()
	if err != nil {
		return DescriptorJSON{}, fmt.Errorf("read tags: %w", err)
	}

	tagIDs := make([]string, 0, tags.Len())
	renderedTags := make([]TagJSON, 0, tags.Len())
	for i := 0; i < tags.Len(); i++ {
		tag := tags.At(i)
		tagIDs = append(tagIDs, fmt.Sprintf("0x%x", tag.Id()))

		rendered, err := renderTag(tag)
		if err != nil {
			return DescriptorJSON{}, err
		}
		renderedTags = append(renderedTags, rendered)
	}

	return DescriptorJSON{
		Quality: desc.Quality().String(),
		TagIDs:  tagIDs,
		Tags:    renderedTags,
		Text:    desc.String(),
	}, nil
}

func renderTag(tag powerbox.PowerboxDescriptor_Tag) (TagJSON, error) {
	rendered := TagJSON{
		ID: fmt.Sprintf("0x%x", tag.Id()),
	}

	ptr, err := tag.Value()
	if err != nil {
		return TagJSON{}, fmt.Errorf("read tag value: %w", err)
	}
	if !ptr.IsValid() {
		return rendered, nil
	}

	switch tag.Id() {
	case identity.Identity_TypeID:
		v := identity.Identity_PowerboxTag{}.DecodeFromPtr(ptr)
		perms, err := v.Permissions()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode identity powerbox tag: %w", err)
		}
		values := make([]bool, perms.Len())
		for i := 0; i < perms.Len(); i++ {
			values[i] = perms.At(i)
		}
		rendered.KnownType = "Identity"
		rendered.Value = map[string]any{"permissions": values}
	case email.EmailSendPort_TypeID:
		v := email.EmailSendPort_PowerboxTag{}.DecodeFromPtr(ptr)
		value := map[string]any{}
		if v.HasFromHint() {
			from, err := v.FromHint()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode email send port tag: %w", err)
			}
			address, err := from.Address()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode email send port fromHint address: %w", err)
			}
			name, err := from.Name()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode email send port fromHint name: %w", err)
			}
			value["fromHint"] = map[string]any{"address": address, "name": name}
		}
		if v.HasListIdHint() {
			listID, err := v.ListIdHint()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode email send port tag: %w", err)
			}
			value["listIdHint"] = listID
		}
		rendered.KnownType = "EmailSendPort"
		rendered.Value = value
	case email.VerifiedEmail_TypeID:
		v := email.VerifiedEmail_PowerboxTag{}.DecodeFromPtr(ptr)
		if !v.HasAddress() {
			return TagJSON{}, fmt.Errorf("decode verified email address: missing field")
		}
		if !v.HasDomain() {
			return TagJSON{}, fmt.Errorf("decode verified email domain: missing field")
		}
		if !v.HasVerifierId() {
			return TagJSON{}, fmt.Errorf("decode verified email verifierId: missing field")
		}
		address, err := v.Address()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode verified email address: %w", err)
		}
		domain, err := v.Domain()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode verified email domain: %w", err)
		}
		verifierID, err := v.VerifierId()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode verified email verifierId: %w", err)
		}
		rendered.KnownType = "VerifiedEmail"
		rendered.Value = map[string]any{
			"address":    address,
			"domain":     domain,
			"verifierId": fmt.Sprintf("%x", verifierID),
		}
	case email.VerifiedEmailSendPort_TypeID:
		v := email.VerifiedEmailSendPort_PowerboxTag{}.DecodeFromPtr(ptr)
		value := map[string]any{}
		if v.HasVerification() {
			verification, err := v.Verification()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode verified email send port tag: %w", err)
			}
			address, err := verification.Address()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode verified email send port verification address: %w", err)
			}
			domain, err := verification.Domain()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode verified email send port verification domain: %w", err)
			}
			verifierID, err := verification.VerifierId()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode verified email send port verification verifierId: %w", err)
			}
			value["verification"] = map[string]any{
				"address":    address,
				"domain":     domain,
				"verifierId": fmt.Sprintf("%x", verifierID),
			}
		}
		if v.HasPort() {
			port, err := v.Port()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode verified email send port tag: %w", err)
			}
			portValue := map[string]any{}
			if port.HasFromHint() {
				from, err := port.FromHint()
				if err != nil {
					return TagJSON{}, fmt.Errorf("decode verified email send port fromHint: %w", err)
				}
				address, err := from.Address()
				if err != nil {
					return TagJSON{}, fmt.Errorf("decode verified email send port fromHint address: %w", err)
				}
				name, err := from.Name()
				if err != nil {
					return TagJSON{}, fmt.Errorf("decode verified email send port fromHint name: %w", err)
				}
				portValue["fromHint"] = map[string]any{"address": address, "name": name}
			}
			if port.HasListIdHint() {
				listID, err := port.ListIdHint()
				if err != nil {
					return TagJSON{}, fmt.Errorf("decode verified email send port listIdHint: %w", err)
				}
				portValue["listIdHint"] = listID
			}
			value["port"] = portValue
		}
		rendered.KnownType = "VerifiedEmailSendPort"
		rendered.Value = value
	case apisession.ApiSession_TypeID:
		v := apisession.ApiSession_PowerboxTag{}.DecodeFromPtr(ptr)
		if !v.HasCanonicalUrl() {
			return TagJSON{}, fmt.Errorf("decode api session canonicalUrl: missing field")
		}
		if !v.HasOauthScopes() {
			return TagJSON{}, fmt.Errorf("decode api session oauthScopes: missing field")
		}
		if !v.HasAuthentication() {
			return TagJSON{}, fmt.Errorf("decode api session authentication: missing field")
		}
		canonicalURL, err := v.CanonicalUrl()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode api session canonicalUrl: %w", err)
		}
		authentication, err := v.Authentication()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode api session authentication: %w", err)
		}
		scopeList, err := v.OauthScopes()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode api session tag: %w", err)
		}
		scopes := make([]string, 0, scopeList.Len())
		for i := 0; i < scopeList.Len(); i++ {
			name, err := scopeList.At(i).Name()
			if err != nil {
				return TagJSON{}, fmt.Errorf("decode api session oauth scope: %w", err)
			}
			scopes = append(scopes, name)
		}
		rendered.KnownType = "ApiSession"
		rendered.Value = map[string]any{
			"canonicalUrl":   canonicalURL,
			"authentication": authentication,
			"oauthScopes":    scopes,
		}
	case ip.IpNetwork_TypeID:
		v := ip.IpNetwork_PowerboxTag{}.DecodeFromPtr(ptr)
		encryption, err := v.Encryption()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode ip network tag: %w", err)
		}
		rendered.KnownType = "IpNetwork"
		rendered.Value = map[string]any{
			"encryption": encryptionWhichString(encryption),
		}
	case grain.UiView_TypeID:
		v := grain.UiView_PowerboxTag{}.DecodeFromPtr(ptr)
		if !v.HasTitle() {
			return TagJSON{}, fmt.Errorf("decode ui view title: missing field")
		}
		title, err := v.Title()
		if err != nil {
			return TagJSON{}, fmt.Errorf("decode ui view tag: %w", err)
		}
		rendered.KnownType = "UiView"
		rendered.Value = map[string]any{"title": title}
	default:
		rendered.Text = ptrText(ptr)
	}

	return rendered, nil
}

func encryptionWhichString(v ip.IpNetwork_PowerboxTag_Encryption) string {
	switch v.Which() {
	case ip.IpNetwork_PowerboxTag_Encryption_Which_none:
		return "none"
	case ip.IpNetwork_PowerboxTag_Encryption_Which_tls:
		return "tls"
	default:
		return "unknown"
	}
}

func ptrText(ptr capnp.Ptr) string {
	if !ptr.IsValid() {
		return ""
	}
	if text := ptr.Text(); text != "" {
		return fmt.Sprintf("text(%q)", text)
	}

	if data := ptr.Data(); data != nil {
		preview := fmt.Sprintf("%x", data)
		if len(preview) > 32 {
			preview = preview[:32] + "..."
		}
		return fmt.Sprintf("data(len=%d hex=%s)", len(data), preview)
	}

	if s := ptr.Struct(); s.IsValid() {
		size := s.Size()
		return fmt.Sprintf(
			"struct(dataWords=%d pointers=%d)",
			uint32(size.DataSize)/8,
			size.PointerCount,
		)
	}

	if l := ptr.List(); l.IsValid() {
		return fmt.Sprintf("list(len=%d)", l.Len())
	}

	if i := ptr.Interface(); i.IsValid() {
		return "interface(capability)"
	}

	return strings.TrimSpace(fmt.Sprintf("%v", ptr))
}
