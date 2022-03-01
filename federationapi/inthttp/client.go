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
	"github.com/opentracing/opentracing-go"
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
	FederationAPIPerformServersAlivePath           = "/federationapi/performServersAlive"
	FederationAPIPerformBroadcastEDUPath           = "/federationapi/performBroadcastEDU"

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
	return &httpFederationInternalAPI{federationSenderURL, httpClient, cache}, nil
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeaveRequest")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformLeaveRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle sending an invite to a remote server.
func (h *httpFederationInternalAPI) PerformInvite(
	ctx context.Context,
	request *api.PerformInviteRequest,
	response *api.PerformInviteResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformInviteRequest")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformInviteRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle starting a peek on a remote server.
func (h *httpFederationInternalAPI) PerformOutboundPeek(
	ctx context.Context,
	request *api.PerformOutboundPeekRequest,
	response *api.PerformOutboundPeekResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformOutboundPeekRequest")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformOutboundPeekRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpFederationInternalAPI) PerformServersAlive(
	ctx context.Context,
	request *api.PerformServersAliveRequest,
	response *api.PerformServersAliveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformServersAlive")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformServersAlivePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryJoinedHostServerNamesInRoom implements FederationInternalAPI
func (h *httpFederationInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostServerNamesInRoom")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIQueryJoinedHostServerNamesInRoomPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoinRequest")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformJoinRequestPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDirectoryLookup")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformDirectoryLookupRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to broadcast an EDU to all servers in rooms we are joined to.
func (h *httpFederationInternalAPI) PerformBroadcastEDU(
	ctx context.Context,
	request *api.PerformBroadcastEDURequest,
	response *api.PerformBroadcastEDUResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformBroadcastEDU")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIPerformBroadcastEDUPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type getUserDevices struct {
	S      gomatrixserverlib.ServerName
	UserID string
	Res    *gomatrixserverlib.RespUserDevices
	Err    *api.FederationClientError
}

func (h *httpFederationInternalAPI) GetUserDevices(
	ctx context.Context, s gomatrixserverlib.ServerName, userID string,
) (gomatrixserverlib.RespUserDevices, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetUserDevices")
	defer span.Finish()

	var result gomatrixserverlib.RespUserDevices
	request := getUserDevices{
		S:      s,
		UserID: userID,
	}
	var response getUserDevices
	apiURL := h.federationAPIURL + FederationAPIGetUserDevicesPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return result, err
	}
	if response.Err != nil {
		return result, response.Err
	}
	return *response.Res, nil
}

type claimKeys struct {
	S           gomatrixserverlib.ServerName
	OneTimeKeys map[string]map[string]string
	Res         *gomatrixserverlib.RespClaimKeys
	Err         *api.FederationClientError
}

func (h *httpFederationInternalAPI) ClaimKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string,
) (gomatrixserverlib.RespClaimKeys, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ClaimKeys")
	defer span.Finish()

	var result gomatrixserverlib.RespClaimKeys
	request := claimKeys{
		S:           s,
		OneTimeKeys: oneTimeKeys,
	}
	var response claimKeys
	apiURL := h.federationAPIURL + FederationAPIClaimKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return result, err
	}
	if response.Err != nil {
		return result, response.Err
	}
	return *response.Res, nil
}

type queryKeys struct {
	S    gomatrixserverlib.ServerName
	Keys map[string][]string
	Res  *gomatrixserverlib.RespQueryKeys
	Err  *api.FederationClientError
}

func (h *httpFederationInternalAPI) QueryKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keys map[string][]string,
) (gomatrixserverlib.RespQueryKeys, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryKeys")
	defer span.Finish()

	var result gomatrixserverlib.RespQueryKeys
	request := queryKeys{
		S:    s,
		Keys: keys,
	}
	var response queryKeys
	apiURL := h.federationAPIURL + FederationAPIQueryKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return result, err
	}
	if response.Err != nil {
		return result, response.Err
	}
	return *response.Res, nil
}

type backfill struct {
	S        gomatrixserverlib.ServerName
	RoomID   string
	Limit    int
	EventIDs []string
	Res      *gomatrixserverlib.Transaction
	Err      *api.FederationClientError
}

func (h *httpFederationInternalAPI) Backfill(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string, limit int, eventIDs []string,
) (gomatrixserverlib.Transaction, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Backfill")
	defer span.Finish()

	request := backfill{
		S:        s,
		RoomID:   roomID,
		Limit:    limit,
		EventIDs: eventIDs,
	}
	var response backfill
	apiURL := h.federationAPIURL + FederationAPIBackfillPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.Transaction{}, response.Err
	}
	return *response.Res, nil
}

type lookupState struct {
	S           gomatrixserverlib.ServerName
	RoomID      string
	EventID     string
	RoomVersion gomatrixserverlib.RoomVersion
	Res         *gomatrixserverlib.RespState
	Err         *api.FederationClientError
}

func (h *httpFederationInternalAPI) LookupState(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion,
) (gomatrixserverlib.RespState, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "LookupState")
	defer span.Finish()

	request := lookupState{
		S:           s,
		RoomID:      roomID,
		EventID:     eventID,
		RoomVersion: roomVersion,
	}
	var response lookupState
	apiURL := h.federationAPIURL + FederationAPILookupStatePath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.RespState{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.RespState{}, response.Err
	}
	return *response.Res, nil
}

type lookupStateIDs struct {
	S       gomatrixserverlib.ServerName
	RoomID  string
	EventID string
	Res     *gomatrixserverlib.RespStateIDs
	Err     *api.FederationClientError
}

func (h *httpFederationInternalAPI) LookupStateIDs(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID, eventID string,
) (gomatrixserverlib.RespStateIDs, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "LookupStateIDs")
	defer span.Finish()

	request := lookupStateIDs{
		S:       s,
		RoomID:  roomID,
		EventID: eventID,
	}
	var response lookupStateIDs
	apiURL := h.federationAPIURL + FederationAPILookupStateIDsPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.RespStateIDs{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.RespStateIDs{}, response.Err
	}
	return *response.Res, nil
}

type lookupMissingEvents struct {
	S           gomatrixserverlib.ServerName
	RoomID      string
	Missing     gomatrixserverlib.MissingEvents
	RoomVersion gomatrixserverlib.RoomVersion
	Res         struct {
		Events []gomatrixserverlib.RawJSON `json:"events"`
	}
	Err *api.FederationClientError
}

func (h *httpFederationInternalAPI) LookupMissingEvents(
	ctx context.Context, s gomatrixserverlib.ServerName, roomID string,
	missing gomatrixserverlib.MissingEvents, roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.RespMissingEvents, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "LookupMissingEvents")
	defer span.Finish()

	request := lookupMissingEvents{
		S:           s,
		RoomID:      roomID,
		Missing:     missing,
		RoomVersion: roomVersion,
	}
	apiURL := h.federationAPIURL + FederationAPILookupMissingEventsPath
	err = httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &request)
	if err != nil {
		return res, err
	}
	if request.Err != nil {
		return res, request.Err
	}
	res.Events = request.Res.Events
	return res, nil
}

type getEvent struct {
	S       gomatrixserverlib.ServerName
	EventID string
	Res     *gomatrixserverlib.Transaction
	Err     *api.FederationClientError
}

func (h *httpFederationInternalAPI) GetEvent(
	ctx context.Context, s gomatrixserverlib.ServerName, eventID string,
) (gomatrixserverlib.Transaction, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetEvent")
	defer span.Finish()

	request := getEvent{
		S:       s,
		EventID: eventID,
	}
	var response getEvent
	apiURL := h.federationAPIURL + FederationAPIGetEventPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.Transaction{}, response.Err
	}
	return *response.Res, nil
}

type getEventAuth struct {
	S           gomatrixserverlib.ServerName
	RoomVersion gomatrixserverlib.RoomVersion
	RoomID      string
	EventID     string
	Res         *gomatrixserverlib.RespEventAuth
	Err         *api.FederationClientError
}

func (h *httpFederationInternalAPI) GetEventAuth(
	ctx context.Context, s gomatrixserverlib.ServerName,
	roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string,
) (gomatrixserverlib.RespEventAuth, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetEventAuth")
	defer span.Finish()

	request := getEventAuth{
		S:           s,
		RoomVersion: roomVersion,
		RoomID:      roomID,
		EventID:     eventID,
	}
	var response getEventAuth
	apiURL := h.federationAPIURL + FederationAPIGetEventAuthPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.RespEventAuth{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.RespEventAuth{}, response.Err
	}
	return *response.Res, nil
}

func (h *httpFederationInternalAPI) QueryServerKeys(
	ctx context.Context, req *api.QueryServerKeysRequest, res *api.QueryServerKeysResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryServerKeys")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIQueryServerKeysPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

type lookupServerKeys struct {
	S           gomatrixserverlib.ServerName
	KeyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp
	ServerKeys  []gomatrixserverlib.ServerKeys
	Err         *api.FederationClientError
}

func (h *httpFederationInternalAPI) LookupServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) ([]gomatrixserverlib.ServerKeys, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "LookupServerKeys")
	defer span.Finish()

	request := lookupServerKeys{
		S:           s,
		KeyRequests: keyRequests,
	}
	var response lookupServerKeys
	apiURL := h.federationAPIURL + FederationAPILookupServerKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return []gomatrixserverlib.ServerKeys{}, err
	}
	if response.Err != nil {
		return []gomatrixserverlib.ServerKeys{}, response.Err
	}
	return response.ServerKeys, nil
}

type eventRelationships struct {
	S       gomatrixserverlib.ServerName
	Req     gomatrixserverlib.MSC2836EventRelationshipsRequest
	RoomVer gomatrixserverlib.RoomVersion
	Res     gomatrixserverlib.MSC2836EventRelationshipsResponse
	Err     *api.FederationClientError
}

func (h *httpFederationInternalAPI) MSC2836EventRelationships(
	ctx context.Context, s gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest,
	roomVersion gomatrixserverlib.RoomVersion,
) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "MSC2836EventRelationships")
	defer span.Finish()

	request := eventRelationships{
		S:       s,
		Req:     r,
		RoomVer: roomVersion,
	}
	var response eventRelationships
	apiURL := h.federationAPIURL + FederationAPIEventRelationshipsPath
	err = httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return res, err
	}
	if response.Err != nil {
		return res, response.Err
	}
	return response.Res, nil
}

type spacesReq struct {
	S             gomatrixserverlib.ServerName
	SuggestedOnly bool
	RoomID        string
	Res           gomatrixserverlib.MSC2946SpacesResponse
	Err           *api.FederationClientError
}

func (h *httpFederationInternalAPI) MSC2946Spaces(
	ctx context.Context, dst gomatrixserverlib.ServerName, roomID string, suggestedOnly bool,
) (res gomatrixserverlib.MSC2946SpacesResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "MSC2946Spaces")
	defer span.Finish()

	request := spacesReq{
		S:             dst,
		SuggestedOnly: suggestedOnly,
		RoomID:        roomID,
	}
	var response spacesReq
	apiURL := h.federationAPIURL + FederationAPISpacesSummaryPath
	err = httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return res, err
	}
	if response.Err != nil {
		return res, response.Err
	}
	return response.Res, nil
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputPublicKey")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIInputPublicKeyPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpFederationInternalAPI) QueryPublicKeys(
	ctx context.Context,
	request *api.QueryPublicKeysRequest,
	response *api.QueryPublicKeysResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryPublicKey")
	defer span.Finish()

	apiURL := h.federationAPIURL + FederationAPIQueryPublicKeyPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
