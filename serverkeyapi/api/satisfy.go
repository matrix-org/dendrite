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
		if _, ok := request.Keys[req.ServerName]; !ok {
			request.Keys[req.ServerName] = map[gomatrixserverlib.KeyID]gomatrixserverlib.PublicKeyLookupResult{}
		}
		request.Keys[req.ServerName][req.KeyID] = res
	}
	return s.InputPublicKeys(ctx, &request, &response)
}

func (s *httpServerKeyInternalAPI) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	request := QueryPublicKeysRequest{}
	response := QueryPublicKeysResponse{}
	for req, ts := range requests {
		if _, ok := request.Requests[req.ServerName]; !ok {
			request.Requests[req.ServerName] = map[gomatrixserverlib.KeyID]gomatrixserverlib.Timestamp{}
		}
		request.Requests[req.ServerName][req.KeyID] = ts
	}
	err := s.QueryPublicKeys(ctx, &request, &response)
	if err != nil {
		return nil, err
	}
	result := map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{}
	for serverName, byServerName := range response.Results {
		for keyID, res := range byServerName {
			key := gomatrixserverlib.PublicKeyLookupRequest{
				ServerName: serverName,
				KeyID:      keyID,
			}
			result[key] = res
		}
	}
	return result, nil
}
