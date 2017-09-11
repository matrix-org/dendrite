package gomatrixserverlib

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/matrix-org/gomatrix"
	"golang.org/x/crypto/ed25519"
)

// An FederationClient is a matrix federation client that adds
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

func (ac *FederationClient) doRequest(r FederationRequest, resBody interface{}) error {
	if err := r.Sign(ac.serverName, ac.serverKeyID, ac.serverPrivateKey); err != nil {
		return err
	}

	req, err := r.HTTPRequest()
	if err != nil {
		return err
	}

	res, err := ac.client.Do(req)
	if res != nil {
		defer res.Body.Close() // nolint: errcheck
	}

	if err != nil {
		return err
	}

	contents, err := ioutil.ReadAll(res.Body)
	if res.StatusCode/100 != 2 { // not 2xx
		// Adapted from https://github.com/matrix-org/gomatrix/blob/master/client.go
		var wrap error
		var respErr gomatrix.RespError
		if _ = json.Unmarshal(contents, &respErr); respErr.ErrCode != "" {
			wrap = respErr
		}

		// If we failed to decode as RespError, don't just drop the HTTP body, include it in the
		// HTTP error instead (e.g proxy errors which return HTML).
		msg := "Failed to " + r.Method() + " JSON to " + r.RequestURI()
		if wrap == nil {
			msg = msg + ": " + string(contents)
		}

		return gomatrix.HTTPError{
			Code:         res.StatusCode,
			Message:      msg,
			WrappedError: wrap,
		}
	}

	if err != nil {
		return err
	}

	if resBody == nil {
		return nil
	}

	return json.Unmarshal(contents, resBody)
}

// SendTransaction sends a transaction
func (ac *FederationClient) SendTransaction(t Transaction) (res RespSend, err error) {
	path := "/_matrix/federation/v1/send/" + string(t.TransactionID) + "/"
	req := NewFederationRequest("PUT", t.Destination, path)
	if err = req.SetContent(t); err != nil {
		return
	}
	err = ac.doRequest(req, &res)
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
func (ac *FederationClient) MakeJoin(s ServerName, roomID, userID string) (res RespMakeJoin, err error) {
	path := "/_matrix/federation/v1/make_join/" +
		url.PathEscape(roomID) + "/" +
		url.PathEscape(userID)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(req, &res)
	return
}

// SendJoin sends a join m.room.member event obtained using MakeJoin via a
// remote matrix server.
// This is used to join a room the local server isn't a member of.
// See https://matrix.org/docs/spec/server_server/unstable.html#joining-rooms
func (ac *FederationClient) SendJoin(s ServerName, event Event) (res RespSendJoin, err error) {
	path := "/_matrix/federation/v1/send_join/" +
		url.PathEscape(event.RoomID()) + "/" +
		url.PathEscape(event.EventID())
	req := NewFederationRequest("PUT", s, path)
	if err = req.SetContent(event); err != nil {
		return
	}
	err = ac.doRequest(req, &res)
	return
}

// ExchangeThirdPartyInvite sends the builder of a m.room.member event of
// "invite" membership derived from a response from invites sent by an identity
// server.
// This is used to exchange a m.room.third_party_invite event for a m.room.member
// one in a room the local server isn't a member of.
func (ac *FederationClient) ExchangeThirdPartyInvite(s ServerName, builder EventBuilder) (err error) {
	path := "/_matrix/federation/v1/exchange_third_party_invite/" +
		url.PathEscape(builder.RoomID)
	req := NewFederationRequest("PUT", s, path)
	if err = req.SetContent(builder); err != nil {
		return
	}
	err = ac.doRequest(req, nil)
	return
}

// LookupState retrieves the room state for a room at an event from a
// remote matrix server as full matrix events.
func (ac *FederationClient) LookupState(s ServerName, roomID, eventID string) (res RespState, err error) {
	path := "/_matrix/federation/v1/state/" +
		url.PathEscape(roomID) +
		"/?event_id=" +
		url.QueryEscape(eventID)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(req, &res)
	return
}

// LookupStateIDs retrieves the room state for a room at an event from a
// remote matrix server as lists of matrix event IDs.
func (ac *FederationClient) LookupStateIDs(s ServerName, roomID, eventID string) (res RespStateIDs, err error) {
	path := "/_matrix/federation/v1/state_ids/" +
		url.PathEscape(roomID) +
		"/?event_id=" +
		url.QueryEscape(eventID)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(req, &res)
	return
}

// LookupRoomAlias looks up a room alias hosted on the remote server.
// The domain part of the roomAlias must match the name of the server it is
// being looked up on.
// If the room alias doesn't exist on the remote server then a 404 gomatrix.HTTPError
// is returned.
func (ac *FederationClient) LookupRoomAlias(s ServerName, roomAlias string) (res RespDirectory, err error) {
	path := "/_matrix/federation/v1/query/directory?room_alias=" +
		url.QueryEscape(roomAlias)
	req := NewFederationRequest("GET", s, path)
	err = ac.doRequest(req, &res)
	return
}
