package internal

import (
	"context"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/dendrite/serverkeyapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

type ServerKeyAPI struct {
	api.ServerKeyInternalAPI

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

func (s *ServerKeyAPI) FetcherName() string {
	return s.DB.FetcherName()
}
