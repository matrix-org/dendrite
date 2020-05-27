package storage

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	FetcherName() string
	FetchKeys(ctx context.Context, requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error)
	StoreKeys(ctx context.Context, keyMap map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult) error
}
