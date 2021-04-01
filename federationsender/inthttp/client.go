package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP API
const (
	FederationSenderQueryJoinedHostServerNamesInRoomPath = "/federationsender/queryJoinedHostServerNamesInRoom"

	FederationSenderPerformDirectoryLookupRequestPath = "/federationsender/performDirectoryLookup"
	FederationSenderPerformJoinRequestPath            = "/federationsender/performJoinRequest"
	FederationSenderPerformLeaveRequestPath           = "/federationsender/performLeaveRequest"
	FederationSenderPerformInviteRequestPath          = "/federationsender/performInviteRequest"
	FederationSenderPerformOutboundPeekRequestPath    = "/federationsender/performOutboundPeekRequest"
	FederationSenderPerformServersAlivePath           = "/federationsender/performServersAlive"
	FederationSenderPerformBroadcastEDUPath           = "/federationsender/performBroadcastEDU"

	FederationSenderGetUserDevicesPath     = "/federationsender/client/getUserDevices"
	FederationSenderClaimKeysPath          = "/federationsender/client/claimKeys"
	FederationSenderQueryKeysPath          = "/federationsender/client/queryKeys"
	FederationSenderBackfillPath           = "/federationsender/client/backfill"
	FederationSenderLookupStatePath        = "/federationsender/client/lookupState"
	FederationSenderLookupStateIDsPath     = "/federationsender/client/lookupStateIDs"
	FederationSenderGetEventPath           = "/federationsender/client/getEvent"
	FederationSenderGetServerKeysPath      = "/federationsender/client/getServerKeys"
	FederationSenderLookupServerKeysPath   = "/federationsender/client/lookupServerKeys"
	FederationSenderEventRelationshipsPath = "/federationsender/client/msc2836eventRelationships"
	FederationSenderSpacesSummaryPath      = "/federationsender/client/msc2946spacesSummary"
)

// NewFederationSenderClient creates a FederationSenderInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewFederationSenderClient(federationSenderURL string, httpClient *http.Client) (api.FederationSenderInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewFederationSenderInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpFederationSenderInternalAPI{federationSenderURL, httpClient}, nil
}

type httpFederationSenderInternalAPI struct {
	federationSenderURL string
	httpClient          *http.Client
}

// Handle an instruction to make_leave & send_leave with a remote server.
func (h *httpFederationSenderInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeaveRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformLeaveRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle sending an invite to a remote server.
func (h *httpFederationSenderInternalAPI) PerformInvite(
	ctx context.Context,
	request *api.PerformInviteRequest,
	response *api.PerformInviteResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformInviteRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformInviteRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle starting a peek on a remote server.
func (h *httpFederationSenderInternalAPI) PerformOutboundPeek(
	ctx context.Context,
	request *api.PerformOutboundPeekRequest,
	response *api.PerformOutboundPeekResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformOutboundPeekRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformOutboundPeekRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpFederationSenderInternalAPI) PerformServersAlive(
	ctx context.Context,
	request *api.PerformServersAliveRequest,
	response *api.PerformServersAliveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformServersAlive")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformServersAlivePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryJoinedHostServerNamesInRoom implements FederationSenderInternalAPI
func (h *httpFederationSenderInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostServerNamesInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostServerNamesInRoomPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoinRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformJoinRequestPath
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
func (h *httpFederationSenderInternalAPI) PerformDirectoryLookup(
	ctx context.Context,
	request *api.PerformDirectoryLookupRequest,
	response *api.PerformDirectoryLookupResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDirectoryLookup")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformDirectoryLookupRequestPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to broadcast an EDU to all servers in rooms we are joined to.
func (h *httpFederationSenderInternalAPI) PerformBroadcastEDU(
	ctx context.Context,
	request *api.PerformBroadcastEDURequest,
	response *api.PerformBroadcastEDUResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformBroadcastEDU")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformBroadcastEDUPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type getUserDevices struct {
	S      gomatrixserverlib.ServerName
	UserID string
	Res    *gomatrixserverlib.RespUserDevices
	Err    *api.FederationClientError
}

func (h *httpFederationSenderInternalAPI) GetUserDevices(
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
	apiURL := h.federationSenderURL + FederationSenderGetUserDevicesPath
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

func (h *httpFederationSenderInternalAPI) ClaimKeys(
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
	apiURL := h.federationSenderURL + FederationSenderClaimKeysPath
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

func (h *httpFederationSenderInternalAPI) QueryKeys(
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
	apiURL := h.federationSenderURL + FederationSenderQueryKeysPath
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

func (h *httpFederationSenderInternalAPI) Backfill(
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
	apiURL := h.federationSenderURL + FederationSenderBackfillPath
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

func (h *httpFederationSenderInternalAPI) LookupState(
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
	apiURL := h.federationSenderURL + FederationSenderLookupStatePath
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

func (h *httpFederationSenderInternalAPI) LookupStateIDs(
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
	apiURL := h.federationSenderURL + FederationSenderLookupStateIDsPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.RespStateIDs{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.RespStateIDs{}, response.Err
	}
	return *response.Res, nil
}

type getEvent struct {
	S       gomatrixserverlib.ServerName
	EventID string
	Res     *gomatrixserverlib.Transaction
	Err     *api.FederationClientError
}

func (h *httpFederationSenderInternalAPI) GetEvent(
	ctx context.Context, s gomatrixserverlib.ServerName, eventID string,
) (gomatrixserverlib.Transaction, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetEvent")
	defer span.Finish()

	request := getEvent{
		S:       s,
		EventID: eventID,
	}
	var response getEvent
	apiURL := h.federationSenderURL + FederationSenderGetEventPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.Transaction{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.Transaction{}, response.Err
	}
	return *response.Res, nil
}

type getServerKeys struct {
	S          gomatrixserverlib.ServerName
	ServerKeys gomatrixserverlib.ServerKeys
	Err        *api.FederationClientError
}

func (h *httpFederationSenderInternalAPI) GetServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName,
) (gomatrixserverlib.ServerKeys, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetServerKeys")
	defer span.Finish()

	request := getServerKeys{
		S: s,
	}
	var response getServerKeys
	apiURL := h.federationSenderURL + FederationSenderGetServerKeysPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return gomatrixserverlib.ServerKeys{}, err
	}
	if response.Err != nil {
		return gomatrixserverlib.ServerKeys{}, response.Err
	}
	return response.ServerKeys, nil
}

type lookupServerKeys struct {
	S           gomatrixserverlib.ServerName
	KeyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp
	ServerKeys  []gomatrixserverlib.ServerKeys
	Err         *api.FederationClientError
}

func (h *httpFederationSenderInternalAPI) LookupServerKeys(
	ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) ([]gomatrixserverlib.ServerKeys, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "LookupServerKeys")
	defer span.Finish()

	request := lookupServerKeys{
		S:           s,
		KeyRequests: keyRequests,
	}
	var response lookupServerKeys
	apiURL := h.federationSenderURL + FederationSenderLookupServerKeysPath
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

func (h *httpFederationSenderInternalAPI) MSC2836EventRelationships(
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
	apiURL := h.federationSenderURL + FederationSenderEventRelationshipsPath
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
	S      gomatrixserverlib.ServerName
	Req    gomatrixserverlib.MSC2946SpacesRequest
	RoomID string
	Res    gomatrixserverlib.MSC2946SpacesResponse
	Err    *api.FederationClientError
}

func (h *httpFederationSenderInternalAPI) MSC2946Spaces(
	ctx context.Context, dst gomatrixserverlib.ServerName, roomID string, r gomatrixserverlib.MSC2946SpacesRequest,
) (res gomatrixserverlib.MSC2946SpacesResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "MSC2946Spaces")
	defer span.Finish()

	request := spacesReq{
		S:      dst,
		Req:    r,
		RoomID: roomID,
	}
	var response spacesReq
	apiURL := h.federationSenderURL + FederationSenderSpacesSummaryPath
	err = httputil.PostJSON(ctx, span, h.httpClient, apiURL, &request, &response)
	if err != nil {
		return res, err
	}
	if response.Err != nil {
		return res, response.Err
	}
	return response.Res, nil
}
