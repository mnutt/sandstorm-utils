package sessioninspect

import (
	"context"
	"strings"
	"testing"

	capnp "capnproto.org/go/capnp/v3"

	apisession "github.com/mnutt/sandstorm-utils/internal/generated/apisession"
	email "github.com/mnutt/sandstorm-utils/internal/generated/email"
	powerbox "github.com/mnutt/sandstorm-utils/internal/generated/powerbox"
	"github.com/mnutt/sandstorm-utils/internal/testcapnp"
)

func TestGetSessionRequestWithClient(t *testing.T) {
	t.Parallel()

	apiTag, err := testcapnp.NewApiSessionTag("https://api.example.com", "bearer", "read", "write")
	if err != nil {
		t.Fatalf("NewApiSessionTag returned error: %v", err)
	}
	identityTag, err := testcapnp.NewIdentityTag(true, false, true)
	if err != nil {
		t.Fatalf("NewIdentityTag returned error: %v", err)
	}
	reqDesc, err := testcapnp.NewDescriptorWithTags(apiTag, identityTag)
	if err != nil {
		t.Fatalf("NewDescriptorWithTags returned error: %v", err)
	}

	client := testcapnp.NewClient(&testcapnp.BridgeServer{
		SessionServer:      &testcapnp.SessionServer{},
		RequestDescriptors: []powerbox.PowerboxDescriptor{reqDesc},
	})

	data, err := GetSessionRequestWithClient(context.Background(), client, "session-1")
	if err != nil {
		t.Fatalf("GetSessionRequestWithClient returned error: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "\"sessionType\": \"request\"") ||
		!strings.Contains(got, "\"knownType\": \"ApiSession\"") ||
		!strings.Contains(got, "\"canonicalUrl\": \"https://api.example.com\"") ||
		!strings.Contains(got, "\"oauthScopes\": [") ||
		!strings.Contains(got, "\"knownType\": \"Identity\"") {
		t.Fatalf("unexpected JSON: %s", got)
	}
}

func TestGetSessionOfferWithClient(t *testing.T) {
	t.Parallel()

	emailTag, err := testcapnp.NewEmailSendPortTag("noreply@example.com", "No Reply", "list.example.com")
	if err != nil {
		t.Fatalf("NewEmailSendPortTag returned error: %v", err)
	}
	viewTag, err := testcapnp.NewUiViewTag("My View")
	if err != nil {
		t.Fatalf("NewUiViewTag returned error: %v", err)
	}
	offerDesc, err := testcapnp.NewDescriptorWithTags(emailTag, viewTag)
	if err != nil {
		t.Fatalf("NewDescriptorWithTags returned error: %v", err)
	}

	client := testcapnp.NewClient(&testcapnp.BridgeServer{
		SessionServer:      &testcapnp.SessionServer{},
		OfferDescriptor:    offerDesc,
		OfferCapabilitySet: true,
	})

	data, err := GetSessionOfferWithClient(context.Background(), client, "session-1")
	if err != nil {
		t.Fatalf("GetSessionOfferWithClient returned error: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "\"sessionType\": \"offer\"") ||
		!strings.Contains(got, "\"knownType\": \"EmailSendPort\"") ||
		!strings.Contains(got, "\"address\": \"noreply@example.com\"") ||
		!strings.Contains(got, "\"knownType\": \"UiView\"") ||
		!strings.Contains(got, "\"present\": true") {
		t.Fatalf("unexpected JSON: %s", got)
	}
}

func TestGetSessionRequestWithUnknownTagTextAndData(t *testing.T) {
	t.Parallel()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	textValue, err := capnp.NewText(seg, "hello")
	if err != nil {
		t.Fatalf("NewText: %v", err)
	}
	dataValue, err := capnp.NewData(seg, []byte{0xde, 0xad, 0xbe, 0xef})
	if err != nil {
		t.Fatalf("NewData: %v", err)
	}

	textTag, err := testcapnp.NewTag(0x1234, textValue.ToPtr())
	if err != nil {
		t.Fatalf("NewTag text: %v", err)
	}
	dataTag, err := testcapnp.NewTag(0x5678, dataValue.ToPtr())
	if err != nil {
		t.Fatalf("NewTag data: %v", err)
	}
	reqDesc, err := testcapnp.NewDescriptorWithTags(textTag, dataTag)
	if err != nil {
		t.Fatalf("NewDescriptorWithTags: %v", err)
	}

	client := testcapnp.NewClient(&testcapnp.BridgeServer{
		SessionServer:      &testcapnp.SessionServer{},
		RequestDescriptors: []powerbox.PowerboxDescriptor{reqDesc},
	})

	data, err := GetSessionRequestWithClient(context.Background(), client, "session-1")
	if err != nil {
		t.Fatalf("GetSessionRequestWithClient returned error: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, `"text": "text(\"hello\")"`) ||
		!strings.Contains(got, `"text": "data(len=4 hex=deadbeef)"`) {
		t.Fatalf("unexpected JSON: %s", got)
	}
}

func TestGetSessionRequestFailsOnMalformedKnownTag(t *testing.T) {
	t.Parallel()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	malformedStruct, err := capnp.NewStruct(seg, capnp.ObjectSize{})
	if err != nil {
		t.Fatalf("NewStruct: %v", err)
	}

	apiTag, err := testcapnp.NewTag(apisession.ApiSession_TypeID, malformedStruct.ToPtr())
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	reqDesc, err := testcapnp.NewDescriptorWithTags(apiTag)
	if err != nil {
		t.Fatalf("NewDescriptorWithTags: %v", err)
	}

	client := testcapnp.NewClient(&testcapnp.BridgeServer{
		SessionServer:      &testcapnp.SessionServer{},
		RequestDescriptors: []powerbox.PowerboxDescriptor{reqDesc},
	})

	_, err = GetSessionRequestWithClient(context.Background(), client, "session-1")
	if err == nil || !strings.Contains(err.Error(), "decode api session canonicalUrl: missing field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSessionOfferFailsOnMalformedKnownTag(t *testing.T) {
	t.Parallel()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	malformedStruct, err := capnp.NewStruct(seg, capnp.ObjectSize{})
	if err != nil {
		t.Fatalf("NewStruct: %v", err)
	}

	emailTag, err := testcapnp.NewTag(email.VerifiedEmail_TypeID, malformedStruct.ToPtr())
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	offerDesc, err := testcapnp.NewDescriptorWithTags(emailTag)
	if err != nil {
		t.Fatalf("NewDescriptorWithTags: %v", err)
	}

	client := testcapnp.NewClient(&testcapnp.BridgeServer{
		SessionServer:      &testcapnp.SessionServer{},
		OfferDescriptor:    offerDesc,
		OfferCapabilitySet: true,
	})

	_, err = GetSessionOfferWithClient(context.Background(), client, "session-1")
	if err == nil || !strings.Contains(err.Error(), "decode verified email address: missing field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPtrTextSummaries(t *testing.T) {
	t.Parallel()

	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	st, err := capnp.NewStruct(seg, capnp.ObjectSize{DataSize: 8, PointerCount: 2})
	if err != nil {
		t.Fatalf("NewStruct: %v", err)
	}
	list, err := capnp.NewPointerList(seg, 3)
	if err != nil {
		t.Fatalf("NewPointerList: %v", err)
	}
	iface := capnp.NewInterface(seg, 0)

	if got := ptrText(st.ToPtr()); got != "struct(dataWords=1 pointers=2)" {
		t.Fatalf("unexpected struct summary: %q", got)
	}
	if got := ptrText(list.ToPtr()); got != "list(len=3)" {
		t.Fatalf("unexpected list summary: %q", got)
	}
	if got := ptrText(iface.ToPtr()); got != "interface(capability)" {
		t.Fatalf("unexpected interface summary: %q", got)
	}
}
