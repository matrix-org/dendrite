package caching

import (
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// FederationCache contains the subset of functions needed for
// a federation event cache.
type FederationCache interface {
	GetFederationQueuedPDU(eventNID int64) (event *types.HeaderedEvent, ok bool)
	StoreFederationQueuedPDU(eventNID int64, event *types.HeaderedEvent)
	EvictFederationQueuedPDU(eventNID int64)

	GetFederationQueuedEDU(eventNID int64) (event *gomatrixserverlib.EDU, ok bool)
	StoreFederationQueuedEDU(eventNID int64, event *gomatrixserverlib.EDU)
	EvictFederationQueuedEDU(eventNID int64)
}

func (c Caches) GetFederationQueuedPDU(eventNID int64) (*types.HeaderedEvent, bool) {
	return c.FederationPDUs.Get(eventNID)
}

func (c Caches) StoreFederationQueuedPDU(eventNID int64, event *types.HeaderedEvent) {
	c.FederationPDUs.Set(eventNID, event)
}

func (c Caches) EvictFederationQueuedPDU(eventNID int64) {
	c.FederationPDUs.Unset(eventNID)
}

func (c Caches) GetFederationQueuedEDU(eventNID int64) (*gomatrixserverlib.EDU, bool) {
	return c.FederationEDUs.Get(eventNID)
}

func (c Caches) StoreFederationQueuedEDU(eventNID int64, event *gomatrixserverlib.EDU) {
	c.FederationEDUs.Set(eventNID, event)
}

func (c Caches) EvictFederationQueuedEDU(eventNID int64) {
	c.FederationEDUs.Unset(eventNID)
}
