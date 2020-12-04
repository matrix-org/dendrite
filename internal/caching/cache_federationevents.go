package caching

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

const (
	FederationEventCacheName       = "federation_event"
	FederationEventCacheMaxEntries = 1024
	FederationEventCacheMutable    = true
)

// FederationEventCache contains the subset of functions needed for
// a federation event cache.
type FederationEventCache interface {
	GetFederationEvent(eventNID int64) (event *gomatrixserverlib.HeaderedEvent, ok bool)
	StoreFederationEvent(eventNID int64, event *gomatrixserverlib.HeaderedEvent)
	EvictFederationEvent(eventNID int64)
}

func (c Caches) GetFederationEvent(eventNID int64) (*gomatrixserverlib.HeaderedEvent, bool) {
	key := fmt.Sprintf("%d", eventNID)
	val, found := c.FederationEvents.Get(key)
	if found && val != nil {
		if event, ok := val.(*gomatrixserverlib.HeaderedEvent); ok {
			return event, true
		}
	}
	return nil, false
}

func (c Caches) StoreFederationEvent(eventNID int64, event *gomatrixserverlib.HeaderedEvent) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Set(key, event)
}

func (c Caches) EvictFederationEvent(eventNID int64) {
	key := fmt.Sprintf("%d", eventNID)
	c.FederationEvents.Unset(key)
}
