package caching

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

const (
	FederationEventCacheName       = "federation_event"
	FederationEventCacheMaxEntries = 256
	FederationEventCacheMutable    = true // to allow use of Unset only
	FederationEventCacheMaxAge     = CacheNoMaxAge
)

// FederationCache contains the subset of functions needed for
// a federation event cache.
type FederationCache interface {
	GetFederationQueuedPDU(eventNID int64) (event *gomatrixserverlib.HeaderedEvent, ok bool)
	StoreFederationQueuedPDU(eventNID int64, event *gomatrixserverlib.HeaderedEvent)
	EvictFederationQueuedPDU(eventNID int64)

	GetFederationQueuedEDU(eventNID int64) (event *gomatrixserverlib.EDU, ok bool)
	StoreFederationQueuedEDU(eventNID int64, event *gomatrixserverlib.EDU)
	EvictFederationQueuedEDU(eventNID int64)
}

func (c Caches) GetFederationQueuedPDU(eventNID int64) (*gomatrixserverlib.HeaderedEvent, bool) {
	key := fmt.Sprintf("%d", eventNID)
	val, found := c.FederationEvents.Get(key)
	if found && val != nil {
		if event, ok := val.(*gomatrixserverlib.HeaderedEvent); ok {
			return event, true
		}
	}
	return nil, false
}

func (c Caches) StoreFederationQueuedPDU(eventNID int64, event *gomatrixserverlib.HeaderedEvent) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Set(key, event)
}

func (c Caches) EvictFederationQueuedPDU(eventNID int64) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Unset(key)
}

func (c Caches) GetFederationQueuedEDU(eventNID int64) (*gomatrixserverlib.EDU, bool) {
	key := fmt.Sprintf("%d", eventNID)
	val, found := c.FederationEvents.Get(key)
	if found && val != nil {
		if event, ok := val.(*gomatrixserverlib.EDU); ok {
			return event, true
		}
	}
	return nil, false
}

func (c Caches) StoreFederationQueuedEDU(eventNID int64, event *gomatrixserverlib.EDU) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Set(key, event)
}

func (c Caches) EvictFederationQueuedEDU(eventNID int64) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Unset(key)
}
