package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

type SigningKeyServerAPI interface {
	gomatrixserverlib.KeyDatabase

	KeyRing() *gomatrixserverlib.KeyRing

	InputPublicKeys(
		ctx context.Context,
		request *InputPublicKeysRequest,
		response *InputPublicKeysResponse,
	) error

	QueryPublicKeys(
		ctx context.Context,
		request *QueryPublicKeysRequest,
		response *QueryPublicKeysResponse,
	) error
}

type QueryPublicKeysRequest struct {
	Requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp `json:"requests"`
}

type QueryPublicKeysResponse struct {
	Results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult `json:"results"`
}

type InputPublicKeysRequest struct {
	Keys map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult `json:"keys"`
}

type InputPublicKeysResponse struct {
}
