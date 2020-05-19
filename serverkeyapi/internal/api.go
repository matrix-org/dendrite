package internal

import (
	"context"

	"github.com/matrix-org/dendrite/common/caching"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/serverkeyapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

type ServerKeyAPI struct {
	gomatrixserverlib.KeyDatabase

	DB             storage.Database
	Cfg            *config.Dendrite
	ImmutableCache caching.ImmutableCache
	OurKeyRing     gomatrixserverlib.KeyRing
	FedClient      *gomatrixserverlib.FederationClient
}

func (s *ServerKeyAPI) KeyRing() *gomatrixserverlib.KeyRing {
	return &s.OurKeyRing
}

func (s *ServerKeyAPI) StoreKeys(
	ctx context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	return s.DB.StoreKeys(ctx, results)
}

func (s *ServerKeyAPI) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	return s.DB.FetchKeys(ctx, requests)
}
