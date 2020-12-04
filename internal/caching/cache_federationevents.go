package caching

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

const (
	FederationEventCacheName       = "federation_event"
	FederationEventCacheMaxEntries = 256
	FederationEventCacheMutable    = true // to allow use of Unset only
)

// FederationSenderCache contains the subset of functions needed for
// a federation event cache.
type FederationSenderCache interface {
	GetFederationSenderQueuedPDU(eventNID int64) (event *gomatrixserverlib.HeaderedEvent, ok bool)
	StoreFederationSenderQueuedPDU(eventNID int64, event *gomatrixserverlib.HeaderedEvent)
	EvictFederationSenderQueuedPDU(eventNID int64)

	GetFederationSenderQueuedEDU(eventNID int64) (event *gomatrixserverlib.EDU, ok bool)
	StoreFederationSenderQueuedEDU(eventNID int64, event *gomatrixserverlib.EDU)
	EvictFederationSenderQueuedEDU(eventNID int64)
}

func (c Caches) GetFederationSenderQueuedPDU(eventNID int64) (*gomatrixserverlib.HeaderedEvent, bool) {
	key := fmt.Sprintf("%d", eventNID)
	val, found := c.FederationEvents.Get(key)
	if found && val != nil {
		if event, ok := val.(*gomatrixserverlib.HeaderedEvent); ok {
			return event, true
		}
	}
	return nil, false
}

func (c Caches) StoreFederationSenderQueuedPDU(eventNID int64, event *gomatrixserverlib.HeaderedEvent) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Set(key, event)
}

func (c Caches) EvictFederationSenderQueuedPDU(eventNID int64) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Unset(key)
}

func (c Caches) GetFederationSenderQueuedEDU(eventNID int64) (*gomatrixserverlib.EDU, bool) {
	key := fmt.Sprintf("%d", eventNID)
	val, found := c.FederationEvents.Get(key)
	if found && val != nil {
		if event, ok := val.(*gomatrixserverlib.EDU); ok {
			return event, true
		}
	}
	return nil, false
}

func (c Caches) StoreFederationSenderQueuedEDU(eventNID int64, event *gomatrixserverlib.EDU) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Set(key, event)
}

func (c Caches) EvictFederationSenderQueuedEDU(eventNID int64) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Unset(key)
}
