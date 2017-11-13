package gomatrixserverlib

import (
	"context"
	"net/http"
	"net/url"

	"golang.org/x/crypto/ed25519"
)

// A FederationClient is a matrix federation client that adds
// "Authorization: X-Matrix" headers to requests that need ed25519 signatures
type FederationClient struct {
	Client
	serverName       ServerName
	serverKeyID      KeyID
	serverPrivateKey ed25519.PrivateKey
}

// NewFederationClient makes a new FederationClient
func NewFederationClient(
	serverName ServerName, keyID KeyID, privateKey ed25519.PrivateKey,
) *FederationClient {
	return &FederationClient{
		Client:           Client{client: http.Client{Transport: newFederationTripper()}},
		serverName:       serverName,
		serverKeyID:      keyID,
		serverPrivateKey: privateKey,
	}
}

func (ac *FederationClient) doRequest(ctx context.Context, r FederationRequest, resBody interface{}) error {
	if err := r.Sign(ac.serverName, ac.serverKeyID, ac.serverPrivateKey); err != nil {
		return err
	}

	req, err := r.HTTPRequest()
	if err != nil {
		return err
	}

	return ac.Client.DoRequestAndParseResponse(ctx, req, resBody)
}

var federationPathPrefix = "/_matrix/federation/v1"

// SendTransaction sends a transaction
func (ac *FederationClient) SendTransaction(
	ctx context.Context, t Transaction,
) (res RespSend, err error) {
	path := federationPathPrefix + "/send/" + string(t.TransactionID) + "/"
	req := NewFederationRequest("PUT", t.Destination, path)
	if err = req.SetContent(t); err != nil {
		return
	}
	err = ac.doRequest(ctx, req, &res)
	return
}

// MakeJoin makes a join m.room.member event for a room on a remote matrix server.
// This is used to join a room the local server isn't a member of.
// We need to query a remote server because if we aren't in the room we don't
// know what to use for the "prev_events" in the join event.
// The remote server should return us a m.room.member event for our local user
// with the "prev_events" filled out.
// If this successfully returns an acceptable event we will sign it with our
// server's key and pass it to SendJoin.
// See https://matrix.org/docs/spec/server_server/unstable.html#joining-rooms
func (ac *FederationClient) MakeJoin(
	ctx context.Context, s ServerName, roomID, userID string,
) (res RespMakeJoin, err error) {
	path := federationPathPrefix + "/make_join/" +
		url.PathEscape(roomID) + "/" +
		url.PathEscape(userID)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(ctx, req, &res)
	return
}

// SendJoin sends a join m.room.member event obtained using MakeJoin via a
// remote matrix server.
// This is used to join a room the local server isn't a member of.
// See https://matrix.org/docs/spec/server_server/unstable.html#joining-rooms
func (ac *FederationClient) SendJoin(
	ctx context.Context, s ServerName, event Event,
) (res RespSendJoin, err error) {
	path := federationPathPrefix + "/send_join/" +
		url.PathEscape(event.RoomID()) + "/" +
		url.PathEscape(event.EventID())
	req := NewFederationRequest("PUT", s, path)
	if err = req.SetContent(event); err != nil {
		return
	}
	err = ac.doRequest(ctx, req, &res)
	return
}

// SendInvite sends an invite m.room.member event to an invited server to be
// signed by it. This is used to invite a user that is not on the local server.
func (ac *FederationClient) SendInvite(
	ctx context.Context, s ServerName, event Event,
) (res RespInvite, err error) {
	path := federationPathPrefix + "/invite/" +
		url.PathEscape(event.RoomID()) + "/" +
		url.PathEscape(event.EventID())
	req := NewFederationRequest("PUT", s, path)
	if err = req.SetContent(event); err != nil {
		return
	}
	err = ac.doRequest(ctx, req, &res)
	return
}

// ExchangeThirdPartyInvite sends the builder of a m.room.member event of
// "invite" membership derived from a response from invites sent by an identity
// server.
// This is used to exchange a m.room.third_party_invite event for a m.room.member
// one in a room the local server isn't a member of.
func (ac *FederationClient) ExchangeThirdPartyInvite(
	ctx context.Context, s ServerName, builder EventBuilder,
) (err error) {
	path := federationPathPrefix + "/exchange_third_party_invite/" +
		url.PathEscape(builder.RoomID)
	req := NewFederationRequest("PUT", s, path)
	if err = req.SetContent(builder); err != nil {
		return
	}
	err = ac.doRequest(ctx, req, nil)
	return
}

// LookupState retrieves the room state for a room at an event from a
// remote matrix server as full matrix events.
func (ac *FederationClient) LookupState(
	ctx context.Context, s ServerName, roomID, eventID string,
) (res RespState, err error) {
	path := federationPathPrefix + "/state/" +
		url.PathEscape(roomID) +
		"/?event_id=" +
		url.QueryEscape(eventID)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(ctx, req, &res)
	return
}

// LookupStateIDs retrieves the room state for a room at an event from a
// remote matrix server as lists of matrix event IDs.
func (ac *FederationClient) LookupStateIDs(
	ctx context.Context, s ServerName, roomID, eventID string,
) (res RespStateIDs, err error) {
	path := federationPathPrefix + "/state_ids/" +
		url.PathEscape(roomID) +
		"/?event_id=" +
		url.QueryEscape(eventID)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(ctx, req, &res)
	return
}

// LookupRoomAlias looks up a room alias hosted on the remote server.
// The domain part of the roomAlias must match the name of the server it is
// being looked up on.
// If the room alias doesn't exist on the remote server then a 404 gomatrix.HTTPError
// is returned.
func (ac *FederationClient) LookupRoomAlias(
	ctx context.Context, s ServerName, roomAlias string,
) (res RespDirectory, err error) {
	path := federationPathPrefix + "/query/directory?room_alias=" +
		url.QueryEscape(roomAlias)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(ctx, req, &res)
	return
}
