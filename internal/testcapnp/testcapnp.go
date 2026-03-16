package testcapnp

import (
	"context"
	"errors"
	"net"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"capnproto.org/go/capnp/v3/server"

	activity "github.com/mnutt/sandstorm-utils/internal/generated/activity"
	apisession "github.com/mnutt/sandstorm-utils/internal/generated/apisession"
	email "github.com/mnutt/sandstorm-utils/internal/generated/email"
	grain "github.com/mnutt/sandstorm-utils/internal/generated/grain"
	hacksession "github.com/mnutt/sandstorm-utils/internal/generated/hacksession"
	identity "github.com/mnutt/sandstorm-utils/internal/generated/identity"
	ip "github.com/mnutt/sandstorm-utils/internal/generated/ip"
	powerbox "github.com/mnutt/sandstorm-utils/internal/generated/powerbox"
	sandstormhttpbridge "github.com/mnutt/sandstorm-utils/internal/generated/sandstormhttpbridge"
	util "github.com/mnutt/sandstorm-utils/internal/generated/util"
	"github.com/mnutt/sandstorm-utils/internal/sandstorm"
)

type PublicIDResult struct {
	PublicID   string
	Hostname   string
	AutoURL    string
	IsDemoUser bool
}

type OpenViewCall struct {
	Path   string
	NewTab bool
}

type ActivityCall struct {
	Path        string
	Type        uint16
	ThreadPath  string
	ThreadTitle string
	Caption     string
	Users       []ActivityUserCall
}

type ActivityUserCall struct {
	Mentioned  bool
	Subscribed bool
	CanView    bool
}

type EmailAddressCall struct {
	Address string
	Name    string
}

type SentEmailCall struct {
	From    EmailAddressCall
	To      []EmailAddressCall
	Cc      []EmailAddressCall
	Bcc     []EmailAddressCall
	ReplyTo *EmailAddressCall
	Subject string
	Text    string
	HTML    string
}

type StayAwakeCall struct {
	Caption      string
	Notification activity.OngoingNotification
	HandleServer *HandleServer
}

type SandstormAPIServer struct {
	mu             sync.Mutex
	StayAwakeCalls []*StayAwakeCall
}

type HandleServer struct {
	mu         sync.Mutex
	Released   bool
	ReleaseCh  chan struct{}
	releasedCh sync.Once
}

type SessionServer struct {
	PublicIDResult PublicIDResult
	UserAddress    string
	UserName       string
	CloseCalls     int
	OpenViewCalls  []OpenViewCall
	ActivityCalls  []ActivityCall
	SentEmails     []SentEmailCall
}

type BridgeServer struct {
	SessionServer      *SessionServer
	APIServer          *SandstormAPIServer
	RequestDescriptors []powerbox.PowerboxDescriptor
	OfferDescriptor    powerbox.PowerboxDescriptor
	OfferCapabilitySet bool
	SavedIdentityIDs   map[string]bool
}

func NewClient(bridge *BridgeServer) *sandstorm.Client {
	return &sandstorm.Client{
		Dial: func(context.Context) (net.Conn, error) {
			serverConn, clientConn := net.Pipe()

			rpc.NewConn(rpc.NewStreamTransport(serverConn), &rpc.Options{
				BootstrapClient: capnp.Client(sandstormhttpbridge.SandstormHttpBridge_ServerToClient(bridge)),
			})

			return clientConn, nil
		},
	}
}

func (s *BridgeServer) GetSandstormApi(ctx context.Context, call sandstormhttpbridge.SandstormHttpBridge_getSandstormApi) error {
	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	if s.APIServer == nil {
		return nil
	}
	api := grain.SandstormApi_ServerToClient(s.APIServer)
	return results.SetApi(api)
}

func (s *BridgeServer) GetSessionContext(ctx context.Context, call sandstormhttpbridge.SandstormHttpBridge_getSessionContext) error {
	results, err := call.AllocResults()
	if err != nil {
		return err
	}

	return results.SetContext(grain.SessionContext(hacksession.HackSessionContext_ServerToClient(s.SessionServer)))
}

func (s *BridgeServer) GetSavedIdentity(_ context.Context, call sandstormhttpbridge.SandstormHttpBridge_getSavedIdentity) error {
	args := call.Args()
	identityID, err := args.IdentityId()
	if err != nil {
		return err
	}

	results, err := call.AllocResults()
	if err != nil {
		return err
	}

	if s.SavedIdentityIDs == nil {
		return nil
	}

	if !s.SavedIdentityIDs[identityID] {
		return nil
	}

	return results.SetIdentity(NewIdentityCapability())
}

func (s *BridgeServer) SaveIdentity(context.Context, sandstormhttpbridge.SandstormHttpBridge_saveIdentity) error {
	return errors.New("not implemented")
}

func (s *BridgeServer) GetSessionRequest(ctx context.Context, call sandstormhttpbridge.SandstormHttpBridge_getSessionRequest) error {
	results, err := call.AllocResults()
	if err != nil {
		return err
	}

	list, err := results.NewRequestInfo(int32(len(s.RequestDescriptors)))
	if err != nil {
		return err
	}

	for i, descriptor := range s.RequestDescriptors {
		if err := list.Set(i, descriptor); err != nil {
			return err
		}
	}

	return nil
}

func (s *BridgeServer) GetSessionOffer(ctx context.Context, call sandstormhttpbridge.SandstormHttpBridge_getSessionOffer) error {
	results, err := call.AllocResults()
	if err != nil {
		return err
	}

	if s.OfferCapabilitySet {
		if err := results.SetOffer(capnp.Client(hacksession.HackSessionContext_ServerToClient(s.SessionServer))); err != nil {
			return err
		}
	}

	if s.OfferDescriptor.IsValid() {
		if err := results.SetDescriptor(s.OfferDescriptor); err != nil {
			return err
		}
	}

	return nil
}

func (s *SessionServer) ObsoleteHttpGet(context.Context, hacksession.HackSessionContext_obsoleteHttpGet) error {
	return errors.New("not implemented")
}

func (s *SessionServer) GetUserAddress(ctx context.Context, call hacksession.HackSessionContext_getUserAddress) error {
	result, err := call.AllocResults()
	if err != nil {
		return err
	}

	if err := result.SetAddress(s.UserAddress); err != nil {
		return err
	}

	return result.SetName(s.UserName)
}

func (s *SessionServer) ObsoleteGenerateApiToken(context.Context, hacksession.HackSessionContext_obsoleteGenerateApiToken) error {
	return errors.New("not implemented")
}

func (s *SessionServer) ObsoleteListApiTokens(context.Context, hacksession.HackSessionContext_obsoleteListApiTokens) error {
	return errors.New("not implemented")
}

func (s *SessionServer) ObsoleteRevokeApiToken(context.Context, hacksession.HackSessionContext_obsoleteRevokeApiToken) error {
	return errors.New("not implemented")
}

func (s *SessionServer) ObsoleteGetIpNetwork(context.Context, hacksession.HackSessionContext_obsoleteGetIpNetwork) error {
	return errors.New("not implemented")
}

func (s *SessionServer) ObsoleteGetIpInterface(context.Context, hacksession.HackSessionContext_obsoleteGetIpInterface) error {
	return errors.New("not implemented")
}

func (s *SessionServer) ObsoleteGetUiViewForEndpoint(context.Context, hacksession.HackSessionContext_obsoleteGetUiViewForEndpoint) error {
	return errors.New("not implemented")
}

func (s *SessionServer) GetSharedPermissions(context.Context, grain.SessionContext_getSharedPermissions) error {
	return errors.New("not implemented")
}

func (s *SessionServer) TieToUser(context.Context, grain.SessionContext_tieToUser) error {
	return errors.New("not implemented")
}

func (s *SessionServer) Offer(context.Context, grain.SessionContext_offer) error {
	return errors.New("not implemented")
}

func (s *SessionServer) Request(context.Context, grain.SessionContext_request) error {
	return errors.New("not implemented")
}

func (s *SessionServer) FulfillRequest(context.Context, grain.SessionContext_fulfillRequest) error {
	return errors.New("not implemented")
}

func (s *SessionServer) Close(ctx context.Context, call grain.SessionContext_close) error {
	s.CloseCalls++
	_, err := call.AllocResults()
	return err
}

func (s *SessionServer) OpenView(ctx context.Context, call grain.SessionContext_openView) error {
	args := call.Args()
	path, err := args.Path()
	if err != nil {
		return err
	}

	s.OpenViewCalls = append(s.OpenViewCalls, OpenViewCall{
		Path:   path,
		NewTab: args.NewTab(),
	})

	_, err = call.AllocResults()
	return err
}

func (s *SessionServer) ClaimRequest(context.Context, grain.SessionContext_claimRequest) error {
	return errors.New("not implemented")
}

func (s *SessionServer) Activity(ctx context.Context, call grain.SessionContext_activity) error {
	args := call.Args()
	event, err := args.Event()
	if err != nil {
		return err
	}

	path, err := event.Path()
	if err != nil {
		return err
	}

	var threadPath string
	var threadTitle string
	if event.HasThread() {
		thread, err := event.Thread()
		if err != nil {
			return err
		}
		threadPath, err = thread.Path()
		if err != nil {
			return err
		}
		title, err := thread.Title()
		if err != nil {
			return err
		}
		threadTitle, err = title.DefaultText()
		if err != nil {
			return err
		}
	}

	var caption string
	if event.HasNotification() {
		notification, err := event.Notification()
		if err != nil {
			return err
		}
		loc, err := notification.Caption()
		if err != nil {
			return err
		}
		caption, err = loc.DefaultText()
		if err != nil {
			return err
		}
	}

	users := []ActivityUserCall(nil)
	if event.HasUsers() {
		list, err := event.Users()
		if err != nil {
			return err
		}
		users = make([]ActivityUserCall, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			item := list.At(i)
			users = append(users, ActivityUserCall{
				Mentioned:  item.Mentioned(),
				Subscribed: item.Subscribed(),
				CanView:    item.CanView(),
			})
		}
	}

	s.ActivityCalls = append(s.ActivityCalls, ActivityCall{
		Path:        path,
		Type:        event.Type(),
		ThreadPath:  threadPath,
		ThreadTitle: threadTitle,
		Caption:     caption,
		Users:       users,
	})

	_, err = call.AllocResults()
	return err
}

type IdentityServer struct{}

func (IdentityServer) GetProfile(context.Context, identity.Identity_getProfile) error {
	return errors.New("not implemented")
}

func NewIdentityCapability() identity.Identity {
	return identity.Identity_ServerToClient(IdentityServer{})
}

func (s *SessionServer) Send(_ context.Context, call email.EmailSendPort_send) error {
	args := call.Args()
	message, err := args.Email()
	if err != nil {
		return err
	}

	from, err := readEmailAddress(message.From)
	if err != nil {
		return err
	}
	to, err := readEmailAddressList(message.To)
	if err != nil {
		return err
	}
	cc, err := readEmailAddressList(message.Cc)
	if err != nil {
		return err
	}
	bcc, err := readEmailAddressList(message.Bcc)
	if err != nil {
		return err
	}

	var replyTo *EmailAddressCall
	if message.HasReplyTo() {
		value, err := readEmailAddress(message.ReplyTo)
		if err != nil {
			return err
		}
		replyTo = &value
	}

	subject, err := message.Subject()
	if err != nil {
		return err
	}
	text, err := message.Text()
	if err != nil {
		return err
	}
	html, err := message.Html()
	if err != nil {
		return err
	}

	s.SentEmails = append(s.SentEmails, SentEmailCall{
		From:    from,
		To:      to,
		Cc:      cc,
		Bcc:     bcc,
		ReplyTo: replyTo,
		Subject: subject,
		Text:    text,
		HTML:    html,
	})

	_, err = call.AllocResults()
	return err
}

func (s *SessionServer) HintAddress(context.Context, email.EmailSendPort_hintAddress) error {
	return errors.New("not implemented")
}

func (s *SessionServer) GetPublicIdImpl(ctx context.Context, call hacksession.HackSessionContext_getPublicId) error {
	result, err := call.AllocResults()
	if err != nil {
		return err
	}

	if err := result.SetPublicId(s.PublicIDResult.PublicID); err != nil {
		return err
	}
	if err := result.SetHostname(s.PublicIDResult.Hostname); err != nil {
		return err
	}
	if err := result.SetAutoUrl(s.PublicIDResult.AutoURL); err != nil {
		return err
	}
	result.SetIsDemoUser(s.PublicIDResult.IsDemoUser)
	return nil
}

func (s *SessionServer) GetPublicId(ctx context.Context, call hacksession.HackSessionContext_getPublicId) error {
	return s.GetPublicIdImpl(ctx, call)
}

func NewDescriptorWithTagIDs(ids ...uint64) (powerbox.PowerboxDescriptor, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor{}, err
	}
	_ = msg

	desc, err := powerbox.NewRootPowerboxDescriptor(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor{}, err
	}
	tags, err := desc.NewTags(int32(len(ids)))
	if err != nil {
		return powerbox.PowerboxDescriptor{}, err
	}
	for i, id := range ids {
		tags.At(i).SetId(id)
	}
	return desc, nil
}

func NewDescriptorWithTags(tags ...powerbox.PowerboxDescriptor_Tag) (powerbox.PowerboxDescriptor, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor{}, err
	}

	desc, err := powerbox.NewRootPowerboxDescriptor(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor{}, err
	}

	list, err := desc.NewTags(int32(len(tags)))
	if err != nil {
		return powerbox.PowerboxDescriptor{}, err
	}

	for i, tag := range tags {
		if err := list.Set(i, tag); err != nil {
			return powerbox.PowerboxDescriptor{}, err
		}
	}

	return desc, nil
}

func NewTag(tagID uint64, value capnp.Ptr) (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	tag, err := powerbox.NewRootPowerboxDescriptor_Tag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	tag.SetId(tagID)
	if err := tag.SetValue(value); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	return tag, nil
}

func NewIdentityTag(permissions ...bool) (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	value, err := identity.NewRootIdentity_PowerboxTag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	bits, err := value.NewPermissions(int32(len(permissions)))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	for i, bit := range permissions {
		bits.Set(i, bit)
	}

	return NewTag(identity.Identity_TypeID, capnp.Struct(value).ToPtr())
}

func NewEmailSendPortTag(address, name, listID string) (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	value, err := email.NewRootEmailSendPort_PowerboxTag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if address != "" || name != "" {
		fromHint, err := value.NewFromHint()
		if err != nil {
			return powerbox.PowerboxDescriptor_Tag{}, err
		}
		if err := fromHint.SetAddress(address); err != nil {
			return powerbox.PowerboxDescriptor_Tag{}, err
		}
		if err := fromHint.SetName(name); err != nil {
			return powerbox.PowerboxDescriptor_Tag{}, err
		}
	}
	if listID != "" {
		if err := value.SetListIdHint(listID); err != nil {
			return powerbox.PowerboxDescriptor_Tag{}, err
		}
	}

	return NewTag(email.EmailSendPort_TypeID, capnp.Struct(value).ToPtr())
}

func NewVerifiedEmailTag(address, domain string, verifierID []byte) (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	value, err := email.NewRootVerifiedEmail_PowerboxTag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if err := value.SetAddress(address); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if err := value.SetDomain(domain); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if err := value.SetVerifierId(verifierID); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	return NewTag(email.VerifiedEmail_TypeID, capnp.Struct(value).ToPtr())
}

func NewApiSessionTag(canonicalURL, authentication string, scopes ...string) (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	value, err := apisession.NewRootApiSession_PowerboxTag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if err := value.SetCanonicalUrl(canonicalURL); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if err := value.SetAuthentication(authentication); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	list, err := value.NewOauthScopes(int32(len(scopes)))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	for i, scope := range scopes {
		entry := list.At(i)
		if err := entry.SetName(scope); err != nil {
			return powerbox.PowerboxDescriptor_Tag{}, err
		}
	}

	return NewTag(apisession.ApiSession_TypeID, capnp.Struct(value).ToPtr())
}

func readEmailAddress(get func() (email.EmailAddress, error)) (EmailAddressCall, error) {
	addr, err := get()
	if err != nil {
		return EmailAddressCall{}, err
	}
	address, err := addr.Address()
	if err != nil {
		return EmailAddressCall{}, err
	}
	name, err := addr.Name()
	if err != nil {
		return EmailAddressCall{}, err
	}
	return EmailAddressCall{Address: address, Name: name}, nil
}

func readEmailAddressList(get func() (email.EmailAddress_List, error)) ([]EmailAddressCall, error) {
	list, err := get()
	if err != nil {
		return nil, err
	}
	values := make([]EmailAddressCall, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		item := list.At(i)
		address, err := item.Address()
		if err != nil {
			return nil, err
		}
		name, err := item.Name()
		if err != nil {
			return nil, err
		}
		values = append(values, EmailAddressCall{Address: address, Name: name})
	}
	return values, nil
}

func NewIpNetworkTagTLS() (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	value, err := ip.NewRootIpNetwork_PowerboxTag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	encryption, err := value.NewEncryption()
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	encryption.SetTls()

	return NewTag(ip.IpNetwork_TypeID, capnp.Struct(value).ToPtr())
}

func NewUiViewTag(title string) (powerbox.PowerboxDescriptor_Tag, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	value, err := grain.NewRootUiView_PowerboxTag(seg)
	if err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}
	if err := value.SetTitle(title); err != nil {
		return powerbox.PowerboxDescriptor_Tag{}, err
	}

	return NewTag(grain.UiView_TypeID, capnp.Struct(value).ToPtr())
}

func NewActivityEvent(path string, eventType uint16, threadPath string, threadTitle string, caption string) (activity.ActivityEvent, error) {
	_, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return activity.ActivityEvent{}, err
	}

	event, err := activity.NewRootActivityEvent(seg)
	if err != nil {
		return activity.ActivityEvent{}, err
	}
	if err := event.SetPath(path); err != nil {
		return activity.ActivityEvent{}, err
	}
	event.SetType(eventType)

	if threadPath != "" || threadTitle != "" {
		thread, err := event.NewThread()
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		if err := thread.SetPath(threadPath); err != nil {
			return activity.ActivityEvent{}, err
		}
		title, err := thread.NewTitle()
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		if err := title.SetDefaultText(threadTitle); err != nil {
			return activity.ActivityEvent{}, err
		}
	}

	if caption != "" {
		notification, err := event.NewNotification()
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		loc, err := notification.NewCaption()
		if err != nil {
			return activity.ActivityEvent{}, err
		}
		if err := loc.SetDefaultText(caption); err != nil {
			return activity.ActivityEvent{}, err
		}
	}

	return event, nil
}

func (s *SandstormAPIServer) DeprecatedPublish(context.Context, grain.SandstormApi_deprecatedPublish) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) DeprecatedRegisterAction(context.Context, grain.SandstormApi_deprecatedRegisterAction) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) ShareCap(context.Context, grain.SandstormApi_shareCap) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) ShareView(context.Context, grain.SandstormApi_shareView) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) Restore(context.Context, grain.SandstormApi_restore) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) Drop(context.Context, grain.SandstormApi_drop) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) Deleted(context.Context, grain.SandstormApi_deleted) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) StayAwake(_ context.Context, call grain.SandstormApi_stayAwake) error {
	args := call.Args()
	displayInfo, err := args.DisplayInfo()
	if err != nil {
		return err
	}
	captionText := ""
	if displayInfo.IsValid() && displayInfo.HasCaption() {
		caption, err := displayInfo.Caption()
		if err != nil {
			return err
		}
		captionText, err = caption.DefaultText()
		if err != nil {
			return err
		}
	}

	handleServer := &HandleServer{ReleaseCh: make(chan struct{})}
	callRecord := &StayAwakeCall{
		Caption:      captionText,
		Notification: args.Notification().AddRef(),
		HandleServer: handleServer,
	}

	s.mu.Lock()
	s.StayAwakeCalls = append(s.StayAwakeCalls, callRecord)
	s.mu.Unlock()

	results, err := call.AllocResults()
	if err != nil {
		return err
	}
	handle := util.Handle_ServerToClient(handleServer)
	return results.SetHandle(handle)
}

func (s *SandstormAPIServer) Save(context.Context, grain.SandstormApi_save) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) BackgroundActivity(context.Context, grain.SandstormApi_backgroundActivity) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) GetIdentityId(context.Context, grain.SandstormApi_getIdentityId) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) Schedule(context.Context, grain.SandstormApi_schedule) error {
	return errors.New("not implemented")
}

func (s *SandstormAPIServer) LastStayAwakeCall() *StayAwakeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.StayAwakeCalls) == 0 {
		return nil
	}
	return s.StayAwakeCalls[len(s.StayAwakeCalls)-1]
}

func (h *HandleServer) Ping(context.Context, util.Handle_ping) error {
	return nil
}

func (h *HandleServer) Shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Released = true
	h.releasedCh.Do(func() {
		close(h.ReleaseCh)
	})
}

var _ server.Shutdowner = (*HandleServer)(nil)
