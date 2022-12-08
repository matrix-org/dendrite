package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

// HTTP paths for the internal HTTP API
const (
	FederationAPIQueryJoinedHostServerNamesInRoomPath = "/federationapi/queryJoinedHostServerNamesInRoom"
	FederationAPIQueryServerKeysPath                  = "/federationapi/queryServerKeys"

	FederationAPIPerformDirectoryLookupRequestPath = "/federationapi/performDirectoryLookup"
	FederationAPIPerformJoinRequestPath            = "/federationapi/performJoinRequest"
	FederationAPIPerformLeaveRequestPath           = "/federationapi/performLeaveRequest"
	FederationAPIPerformInviteRequestPath          = "/federationapi/performInviteRequest"
	FederationAPIPerformOutboundPeekRequestPath    = "/federationapi/performOutboundPeekRequest"
	FederationAPIPerformBroadcastEDUPath           = "/federationapi/performBroadcastEDU"
	FederationAPIPerformWakeupServers              = "/federationapi/performWakeupServers"

	FederationAPIGetUserDevicesPath      = "/federationapi/client/getUserDevices"
	FederationAPIClaimKeysPath           = "/federationapi/client/claimKeys"
	FederationAPIQueryKeysPath           = "/federationapi/client/queryKeys"
	FederationAPIBackfillPath            = "/federationapi/client/backfill"
	FederationAPILookupStatePath         = "/federationapi/client/lookupState"
	FederationAPILookupStateIDsPath      = "/federationapi/client/lookupStateIDs"
	FederationAPILookupMissingEventsPath = "/federationapi/client/lookupMissingEvents"
	FederationAPIGetEventPath            = "/federationapi/client/getEvent"
	FederationAPILookupServerKeysPath    = "/federationapi/client/lookupServerKeys"
	FederationAPIEventRelationshipsPath  = "/federationapi/client/msc2836eventRelationships"
	FederationAPISpacesSummaryPath       = "/federationapi/client/msc2946spacesSummary"
	FederationAPIGetEventAuthPath        = "/federationapi/client/getEventAuth"

	FederationAPIInputPublicKeyPath = "/federationapi/inputPublicKey"
	FederationAPIQueryPublicKeyPath = "/federationapi/queryPublicKey"
)

// NewFederationAPIClient creates a FederationInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewFederationAPIClient(federationSenderURL string, httpClient *http.Client, cache caching.ServerKeyCache) (api.FederationInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewFederationInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpFederationInternalAPI{
		federationAPIURL: federationSenderURL,
		httpClient:       httpClient,
		cache:            cache,
	}, nil
}

type httpFederationInternalAPI struct {
	federationAPIURL string
	httpClient       *http.Client
	cache            caching.ServerKeyCache
}

// Handle an instruction to make_leave & send_leave with a remote server.
func (h *httpFederationInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformLeave", h.federationAPIURL+FederationAPIPerformLeaveRequestPath,
		h.httpClient, ctx, request, response,
	)
}

// Handle sending an invite to a remote server.
func (h *httpFederationInternalAPI) PerformInvite(
	ctx context.Context,
	request *api.PerformInviteRequest,
	response *api.PerformInviteResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformInvite", h.federationAPIURL+FederationAPIPerformInviteRequestPath,
		h.httpClient, ctx, request, response,
	)
}

// Handle starting a peek on a remote server.
func (h *httpFederationInternalAPI) PerformOutboundPeek(
	ctx context.Context,
	request *api.PerformOutboundPeekRequest,
	response *api.PerformOutboundPeekResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformOutboundPeek", h.federationAPIURL+FederationAPIPerformOutboundPeekRequestPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryJoinedHostServerNamesInRoom implements FederationInternalAPI
func (h *httpFederationInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryJoinedHostServerNamesInRoom", h.federationAPIURL+FederationAPIQueryJoinedHostServerNamesInRoomPath,
		h.httpClient, ctx, request, response,
	)
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) {
	if err := httputil.CallInternalRPCAPI(
		"PerformJoinRequest", h.federationAPIURL+FederationAPIPerformJoinRequestPath,
		h.httpClient, ctx, request, response,
	); err != nil {
		response.LastError = &gomatrix.HTTPError{
			Message:      err.Error(),
			Code:         0,
			WrappedError: err,
		}
	}
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationInternalAPI) PerformDirectoryLookup(
	ctx context.Context,
	request *api.PerformDirectoryLookupRequest,
	response *api.PerformDirectoryLookupResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformDirectoryLookup", h.federationAPIURL+FederationAPIPerformDirectoryLookupRequestPath,
		h.httpClient, ctx, request, response,
	)
}

// Handle an instruction to broadcast an EDU to all servers in rooms we are joined to.
func (h *httpFederationInternalAPI) PerformBroadcastEDU(
	ctx context.Context,
	request *api.PerformBroadcastEDURequest,
	response *api.PerformBroadcastEDUResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformBroadcastEDU", h.federationAPIURL+FederationAPIPerformBroadcastEDUPath,
		h.httpClient, ctx, request, response,
	)
}

// Handle an instruction to remove the respective servers from being blacklisted.
func (h *httpFederationInternalAPI) PerformWakeupServers(
	ctx context.Context,
	request *api.PerformWakeupServersRequest,
	response *api.PerformWakeupServersResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformWakeupServers", h.federationAPIURL+FederationAPIPerformWakeupServers,
		h.httpClient, ctx, request, response,
	)
}

type getUserDevices struct {
	S      gomatrixserverlib.ServerName
	Origin gomatrixserverlib.ServerName
	UserID string
}

func (h *httpFederationInternalAPI) GetUserDevices(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, userID string,
) (gomatrixserverlib.RespUserDevices, error) {
	return httputil.CallInternalProxyAPI[getUserDevices, gomatrixserverlib.RespUserDevices, *api.FederationClientError](
		"GetUserDevices", h.federationAPIURL+FederationAPIGetUserDevicesPath, h.httpClient,
		ctx, &getUserDevices{
			S:      s,
			Origin: origin,
			UserID: userID,
		},
	)
}

type claimKeys struct {
	S           gomatrixserverlib.ServerName
	Origin      gomatrixserverlib.ServerName
	OneTimeKeys map[string]map[string]string
}

func (h *httpFederationInternalAPI) ClaimKeys(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string,
) (gomatrixserverlib.RespClaimKeys, error) {
	return httputil.CallInternalProxyAPI[claimKeys, gomatrixserverlib.RespClaimKeys, *api.FederationClientError](
		"ClaimKeys", h.federationAPIURL+FederationAPIClaimKeysPath, h.httpClient,
		ctx, &claimKeys{
			S:           s,
			Origin:      origin,
			OneTimeKeys: oneTimeKeys,
		},
	)
}

type queryKeys struct {
	S      gomatrixserverlib.ServerName
	Origin gomatrixserverlib.ServerName
	Keys   map[string][]string
}

func (h *httpFederationInternalAPI) QueryKeys(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, keys map[string][]string,
) (gomatrixserverlib.RespQueryKeys, error) {
	return httputil.CallInternalProxyAPI[queryKeys, gomatrixserverlib.RespQueryKeys, *api.FederationClientError](
		"QueryKeys", h.federationAPIURL+FederationAPIQueryKeysPath, h.httpClient,
		ctx, &queryKeys{
			S:      s,
			Origin: origin,
			Keys:   keys,
		},
	)
}

type backfill struct {
	S        gomatrixserverlib.ServerName
	Origin   gomatrixserverlib.ServerName
	RoomID   string
	Limit    int
	EventIDs []string
}

func (h *httpFederationInternalAPI) Backfill(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string, limit int, eventIDs []string,
) (gomatrixserverlib.Transaction, error) {
	return httputil.CallInternalProxyAPI[backfill, gomatrixserverlib.Transaction, *api.FederationClientError](
		"Backfill", h.federationAPIURL+FederationAPIBackfillPath, h.httpClient,
		ctx, &backfill{
			S:        s,
			Origin:   origin,
			RoomID:   roomID,
			Limit:    limit,
			EventIDs: eventIDs,
		},
	)
}

type lookupState struct {
	S           gomatrixserverlib.ServerName
	Origin      gomatrixserverlib.ServerName
	RoomID      string
	EventID     string
	RoomVersion gomatrixserverlib.RoomVersion
}

func (h *httpFederationInternalAPI) LookupState(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion,
) (gomatrixserverlib.RespState, error) {
	return httputil.CallInternalProxyAPI[lookupState, gomatrixserverlib.RespState, *api.FederationClientError](
		"LookupState", h.federationAPIURL+FederationAPILookupStatePath, h.httpClient,
		ctx, &lookupState{
			S:           s,
			Origin:      origin,
			RoomID:      roomID,
			EventID:     eventID,
			RoomVersion: roomVersion,
		},
	)
}

type lookupStateIDs struct {
	S       gomatrixserverlib.ServerName
	Origin  gomatrixserverlib.ServerName
	RoomID  string
	EventID string
}

func (h *httpFederationInternalAPI) LookupStateIDs(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID, eventID string,
) (gomatrixserverlib.RespStateIDs, error) {
	return httputil.CallInternalProxyAPI[lookupStateIDs, gomatrixserverlib.RespStateIDs, *api.FederationClientError](
		"LookupStateIDs", h.federationAPIURL+FederationAPILookupStateIDsPath, h.httpClient,
		ctx, &lookupStateIDs{
			S:       s,
			Origin:  origin,
			RoomID:  roomID,
			EventID: eventID,
		},
	)
}

type lookupMissingEvents struct {
	S           gomatrixserverlib.ServerName
	Origin      gomatrixserverlib.ServerName
	RoomID      string
	Missing     gomatrixserverlib.MissingEvents
	RoomVersion gomatrixserverlib.RoomVersion
}

func (h *httpFederationInternalAPI) LookupMissingEvents(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string,
	missing gomatrixserverlib.MissingEvents, roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.RespMissingEvents, err error) {
	return httputil.CallInternalProxyAPI[lookupMissingEvents, gomatrixserverlib.RespMissingEvents, *api.FederationClientError](
		"LookupMissingEvents", h.federationAPIURL+FederationAPILookupMissingEventsPath, h.httpClient,
		ctx, &lookupMissingEvents{
			S:           s,
			Origin:      origin,
			RoomID:      roomID,
			Missing:     missing,
			RoomVersion: roomVersion,
		},
	)
}

type getEvent struct {
	S       gomatrixserverlib.ServerName
	Origin  gomatrixserverlib.ServerName
	EventID string
}

func (h *httpFederationInternalAPI) GetEvent(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, eventID string,
) (gomatrixserverlib.Transaction, error) {
	return httputil.CallInternalProxyAPI[getEvent, gomatrixserverlib.Transaction, *api.FederationClientError](
		"GetEvent", h.federationAPIURL+FederationAPIGetEventPath, h.httpClient,
		ctx, &getEvent{
			S:       s,
			Origin:  origin,
			EventID: eventID,
		},
	)
}

type getEventAuth struct {
	S           gomatrixserverlib.ServerName
	Origin      gomatrixserverlib.ServerName
	RoomVersion gomatrixserverlib.RoomVersion
	RoomID      string
	EventID     string
}

func (h *httpFederationInternalAPI) GetEventAuth(
	ctx context.Context, origin, s gomatrixserverlib.ServerName,
	roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string,
) (gomatrixserverlib.RespEventAuth, error) {
	return httputil.CallInternalProxyAPI[getEventAuth, gomatrixserverlib.RespEventAuth, *api.FederationClientError](
		"GetEventAuth", h.federationAPIURL+FederationAPIGetEventAuthPath, h.httpClient,
		ctx, &getEventAuth{
			S:           s,
			Origin:      origin,
			RoomVersion: roomVersion,
			RoomID:      roomID,
			EventID:     eventID,
		},
	)
}

func (h *httpFederationInternalAPI) QueryServerKeys(
	ctx context.Context, req *api.QueryServerKeysRequest, res *api.QueryServerKeysResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryServerKeys", h.federationAPIURL+FederationAPIQueryServerKeysPath,
		h.httpClient, ctx, req, res,
	)
}

type lookupServerKeys struct {
	S           gomatrixserverlib.ServerName
	KeyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp
}

func (h *httpFederationInternalAPI) LookupServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) ([]gomatrixserverlib.ServerKeys, error) {
	return httputil.CallInternalProxyAPI[lookupServerKeys, []gomatrixserverlib.ServerKeys, *api.FederationClientError](
		"LookupServerKeys", h.federationAPIURL+FederationAPILookupServerKeysPath, h.httpClient,
		ctx, &lookupServerKeys{
			S:           s,
			KeyRequests: keyRequests,
		},
	)
}

type eventRelationships struct {
	S       gomatrixserverlib.ServerName
	Origin  gomatrixserverlib.ServerName
	Req     gomatrixserverlib.MSC2836EventRelationshipsRequest
	RoomVer gomatrixserverlib.RoomVersion
}

func (h *httpFederationInternalAPI) MSC2836EventRelationships(
	ctx context.Context, origin, s gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest,
	roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error) {
	return httputil.CallInternalProxyAPI[eventRelationships, gomatrixserverlib.MSC2836EventRelationshipsResponse, *api.FederationClientError](
		"MSC2836EventRelationships", h.federationAPIURL+FederationAPIEventRelationshipsPath, h.httpClient,
		ctx, &eventRelationships{
			S:       s,
			Origin:  origin,
			Req:     r,
			RoomVer: roomVersion,
		},
	)
}

type spacesReq struct {
	S             gomatrixserverlib.ServerName
	Origin        gomatrixserverlib.ServerName
	SuggestedOnly bool
	RoomID        string
}

func (h *httpFederationInternalAPI) MSC2946Spaces(
	ctx context.Context, origin, dst gomatrixserverlib.ServerName, roomID string, suggestedOnly bool,
) (res gomatrixserverlib.MSC2946SpacesResponse, err error) {
	return httputil.CallInternalProxyAPI[spacesReq, gomatrixserverlib.MSC2946SpacesResponse, *api.FederationClientError](
		"MSC2836EventRelationships", h.federationAPIURL+FederationAPISpacesSummaryPath, h.httpClient,
		ctx, &spacesReq{
			S:             dst,
			Origin:        origin,
			SuggestedOnly: suggestedOnly,
			RoomID:        roomID,
		},
	)
}

func (s *httpFederationInternalAPI) KeyRing() *gomatrixserverlib.KeyRing {
	// This is a bit of a cheat - we tell gomatrixserverlib that this API is
	// both the key database and the key fetcher. While this does have the
	// rather unfortunate effect of preventing gomatrixserverlib from handling
	// key fetchers directly, we can at least reimplement this behaviour on
	// the other end of the API.
	return &gomatrixserverlib.KeyRing{
		KeyDatabase: s,
		KeyFetchers: []gomatrixserverlib.KeyFetcher{},
	}
}

func (s *httpFederationInternalAPI) FetcherName() string {
	return "httpServerKeyInternalAPI"
}

func (s *httpFederationInternalAPI) StoreKeys(
	_ context.Context,
	results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult,
) error {
	// Run in a background context - we don't want to stop this work just
	// because the caller gives up waiting.
	ctx := context.Background()
	request := api.InputPublicKeysRequest{
		Keys: make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult),
	}
	response := api.InputPublicKeysResponse{}
	for req, res := range results {
		request.Keys[req] = res
		s.cache.StoreServerKey(req, res)
	}
	return s.InputPublicKeys(ctx, &request, &response)
}

func (s *httpFederationInternalAPI) FetchKeys(
	_ context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	// Run in a background context - we don't want to stop this work just
	// because the caller gives up waiting.
	ctx := context.Background()
	result := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
	request := api.QueryPublicKeysRequest{
		Requests: make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp),
	}
	response := api.QueryPublicKeysResponse{
		Results: make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult),
	}
	for req, ts := range requests {
		if res, ok := s.cache.GetServerKey(req, ts); ok {
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
		s.cache.StoreServerKey(req, res)
	}
	return result, nil
}

func (h *httpFederationInternalAPI) InputPublicKeys(
	ctx context.Context,
	request *api.InputPublicKeysRequest,
	response *api.InputPublicKeysResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"InputPublicKey", h.federationAPIURL+FederationAPIInputPublicKeyPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpFederationInternalAPI) QueryPublicKeys(
	ctx context.Context,
	request *api.QueryPublicKeysRequest,
	response *api.QueryPublicKeysResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryPublicKeys", h.federationAPIURL+FederationAPIQueryPublicKeyPath,
		h.httpClient, ctx, request, response,
	)
}
