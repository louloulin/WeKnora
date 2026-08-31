package authz

// WireOptions configures which adapters the default composite
// registers. Tests pass a smaller set; production passes the full
// bag. A nil field means the matching adapter is omitted.
//
// Adapters that need paired lookups (KB / wiki page / agent) only
// register when BOTH halves are present so a half-configured
// adapter cannot serve partial decisions. This is enforced in
// NewAuthZComposite.
type WireOptions struct {
	// NotificationRecipient registers the notification adapter.
	NotificationRecipient NotificationRecipientLookup
	// ChatMessageSessionOwner registers the chat-message adapter.
	ChatMessageSessionOwner ChatMessageSessionOwnerLookup
	// KBCreator + KBShare together register the KB adapter.
	KBCreator KBCreatorLookup
	KBShare   KBShareLookup
	// WikiPageResolve + WikiPageOwner together register the
	// wiki-page adapter.
	WikiPageResolve WikiPageResolveLookup
	WikiPageOwner   WikiPageOwnerLookup
	// AgentCreator + AgentShare together register the agent adapter.
	AgentCreator AgentCreatorLookup
	AgentShare   AgentShareLookup
}

// NewAuthZComposite builds the production composite with the
// tenant-role adapter always on and any provided adapters layered
// on top. New adapters are appended here as their services
// stabilise; this is the single registration point.
func NewAuthZComposite(opts WireOptions) Checker {
	adapters := []Adapter{NewTenantRoleAdapter()}
	if opts.NotificationRecipient != nil {
		adapters = append(adapters, NewNotificationAdapter(opts.NotificationRecipient))
	}
	if opts.ChatMessageSessionOwner != nil {
		adapters = append(adapters, NewChatMessageAdapter(opts.ChatMessageSessionOwner))
	}
	if opts.KBCreator != nil && opts.KBShare != nil {
		adapters = append(adapters, NewKBAdapter(opts.KBCreator, opts.KBShare))
	}
	if opts.WikiPageResolve != nil && opts.WikiPageOwner != nil {
		adapters = append(adapters, NewWikiPageAdapter(opts.WikiPageResolve, opts.WikiPageOwner))
	}
	if opts.AgentCreator != nil && opts.AgentShare != nil {
		adapters = append(adapters, NewAgentAdapter(opts.AgentCreator, opts.AgentShare))
	}
	return NewCompositeChecker(adapters...)
}
