package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

func (s *httpServerKeyInternalAPI) FetcherName() string {
	return "httpServerKeyInternalAPI"
}

func (s *httpServerKeyInternalAPI) StoreKeys(
	ctx context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	request := InputPublicKeysRequest{}
	response := InputPublicKeysResponse{}
	for req, res := range results {
		request.Keys[req] = res
		s.immutableCache.StoreServerKey(req, res)
	}
	return s.InputPublicKeys(ctx, &request, &response)
}

func (s *httpServerKeyInternalAPI) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	result := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
	request := QueryPublicKeysRequest{}
	response := QueryPublicKeysResponse{}
	for req, ts := range requests {
		if res, ok := s.immutableCache.GetServerKey(req); ok {
			result[req] = res
			continue
		}
		request.Requests[req] = ts
	}
	err := s.QueryPublicKeys(ctx, &request, &response)
	if err != nil {
		return nil, err
	}
	for req, res := range response.Results {
		result[req] = res
	}
	return result, nil
}
