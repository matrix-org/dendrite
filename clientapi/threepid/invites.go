// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package threepid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// MembershipRequest represents the body of an incoming POST request
// on /rooms/{roomID}/(join|kick|ban|unban|leave|invite)
type MembershipRequest struct {
	UserID   string `json:"user_id"`
	Reason   string `json:"reason"`
	IDServer string `json:"id_server"`
	Medium   string `json:"medium"`
	Address  string `json:"address"`
}

// idServerLookupResponse represents the response described at https://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-identity-api-v1-lookup
type idServerLookupResponse struct {
	TS         int64                        `json:"ts"`
	NotBefore  int64                        `json:"not_before"`
	NotAfter   int64                        `json:"not_after"`
	Medium     string                       `json:"medium"`
	Address    string                       `json:"address"`
	MXID       string                       `json:"mxid"`
	Signatures map[string]map[string]string `json:"signatures"`
}

// idServerLookupResponse represents the response described at https://matrix.org/docs/spec/client_server/r0.2.0.html#invitation-storage
type idServerStoreInviteResponse struct {
	PublicKey   string                        `json:"public_key"`
	Token       string                        `json:"token"`
	DisplayName string                        `json:"display_name"`
	PublicKeys  []gomatrixserverlib.PublicKey `json:"public_keys"`
}

var (
	// ErrMissingParameter is the error raised if a request for 3PID invite has
	// an incomplete body
	ErrMissingParameter = errors.New("'address', 'id_server' and 'medium' must all be supplied")
	// ErrNotTrusted is the error raised if an identity server isn't in the list
	// of trusted servers in the configuration file.
	ErrNotTrusted = errors.New("untrusted server")
)

// CheckAndProcessInvite analyses the body of an incoming membership request.
// If the fields relative to a third-party-invite are all supplied, lookups the
// matching Matrix ID from the given identity server. If no Matrix ID is
// associated to the given 3PID, asks the identity server to store the invite
// and emit a "m.room.third_party_invite" event.
// Returns a representation of the HTTP response to send to the user.
// Returns a representation of a non-200 HTTP response if something went wrong
// in the process, or if some 3PID fields aren't supplied but others are.
// If none of the 3PID-specific fields are supplied, or if a Matrix ID is
// supplied by the identity server, returns nil to indicate that the request
// must be processed as a non-3PID membership request. In the latter case,
// fills the Matrix ID in the request body so a normal invite membership event
// can be emitted.
func CheckAndProcessInvite(
	ctx context.Context,
	device *userapi.Device, body *MembershipRequest, cfg *config.ClientAPI,
	rsAPI api.ClientRoomserverAPI, db userapi.ClientUserAPI,
	roomID string,
	evTime time.Time,
) (inviteStoredOnIDServer bool, err error) {
	if body.Address == "" && body.IDServer == "" && body.Medium == "" {
		// If none of the 3PID-specific fields are supplied, it's a standard invite
		// so return nil for it to be processed as such
		return
	} else if body.Address == "" || body.IDServer == "" || body.Medium == "" {
		// If at least one of the 3PID-specific fields is supplied but not all
		// of them, return an error
		err = ErrMissingParameter
		return
	}

	lookupRes, storeInviteRes, err := queryIDServer(ctx, db, cfg, device, body, roomID)
	if err != nil {
		return
	}

	if lookupRes.MXID == "" {
		// No Matrix ID could be found for this 3PID, meaning that a
		// "m.room.third_party_invite" have to be emitted from the data in
		// storeInviteRes.
		err = emit3PIDInviteEvent(
			ctx, body, storeInviteRes, device, roomID, cfg, rsAPI, evTime,
		)
		inviteStoredOnIDServer = err == nil

		return
	}

	// A Matrix ID have been found: set it in the body request and let the process
	// continue to create a "m.room.member" event with an "invite" membership
	body.UserID = lookupRes.MXID

	return
}

// queryIDServer handles all the requests to the identity server, starting by
// looking up the given 3PID on the given identity server.
// If the lookup returned a Matrix ID, checks if the current time is within the
// time frame in which the 3PID-MXID association is known to be valid, and checks
// the response's signatures. If one of the checks fails, returns an error.
// If the lookup didn't return a Matrix ID, asks the identity server to store
// the invite and to respond with a token.
// Returns a representation of the response for both cases.
// Returns an error if a check or a request failed.
func queryIDServer(
	ctx context.Context,
	userAPI userapi.ClientUserAPI, cfg *config.ClientAPI, device *userapi.Device,
	body *MembershipRequest, roomID string,
) (lookupRes *idServerLookupResponse, storeInviteRes *idServerStoreInviteResponse, err error) {
	if err = isTrusted(body.IDServer, cfg); err != nil {
		return
	}

	// Lookup the 3PID
	lookupRes, err = queryIDServerLookup(ctx, body)
	if err != nil {
		return
	}

	if lookupRes.MXID == "" {
		// No Matrix ID matches with the given 3PID, ask the server to store the
		// invite and return a token
		storeInviteRes, err = queryIDServerStoreInvite(ctx, userAPI, cfg, device, body, roomID)
		return
	}

	// A Matrix ID matches with the given 3PID
	// Get timestamp in milliseconds to compare it with the timestamps provided
	// by the identity server
	now := time.Now().UnixNano() / 1000000
	if lookupRes.NotBefore > now || now > lookupRes.NotAfter {
		// If the current timestamp isn't in the time frame in which the association
		// is known to be valid, re-run the query
		return queryIDServer(ctx, userAPI, cfg, device, body, roomID)
	}

	// Check the request signatures and send an error if one isn't valid
	if err = checkIDServerSignatures(ctx, body, lookupRes); err != nil {
		return
	}

	return
}

// queryIDServerLookup sends a response to the identity server on /_matrix/identity/api/v1/lookup
// and returns the response as a structure.
// Returns an error if the request failed to send or if the response couldn't be parsed.
func queryIDServerLookup(ctx context.Context, body *MembershipRequest) (*idServerLookupResponse, error) {
	address := url.QueryEscape(body.Address)
	requestURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/lookup?medium=%s&address=%s", body.IDServer, body.Medium, address)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		// TODO: Log the error supplied with the identity server?
		errMgs := fmt.Sprintf("Failed to ask %s to store an invite for %s", body.IDServer, body.Address)
		return nil, errors.New(errMgs)
	}

	var res idServerLookupResponse
	err = json.NewDecoder(resp.Body).Decode(&res)
	return &res, err
}

// queryIDServerStoreInvite sends a response to the identity server on /_matrix/identity/api/v1/store-invite
// and returns the response as a structure.
// Returns an error if the request failed to send or if the response couldn't be parsed.
func queryIDServerStoreInvite(
	ctx context.Context,
	userAPI userapi.ClientUserAPI, cfg *config.ClientAPI, device *userapi.Device,
	body *MembershipRequest, roomID string,
) (*idServerStoreInviteResponse, error) {
	// Retrieve the sender's profile to get their display name
	localpart, serverName, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return nil, err
	}

	var profile *authtypes.Profile
	if cfg.Matrix.IsLocalServerName(serverName) {
		res := &userapi.QueryProfileResponse{}
		err = userAPI.QueryProfile(ctx, &userapi.QueryProfileRequest{UserID: device.UserID}, res)
		if err != nil {
			return nil, err
		}
		profile = &authtypes.Profile{
			Localpart:   localpart,
			DisplayName: res.DisplayName,
			AvatarURL:   res.AvatarURL,
		}

	} else {
		profile = &authtypes.Profile{}
	}

	client := http.Client{}

	data := url.Values{}
	data.Add("medium", body.Medium)
	data.Add("address", body.Address)
	data.Add("room_id", roomID)
	data.Add("sender", device.UserID)
	data.Add("sender_display_name", profile.DisplayName)
	// TODO: Also send:
	//      - The room name (room_name)
	//      - The room's avatar url (room_avatar_url)
	//      See https://github.com/matrix-org/sydent/blob/master/sydent/http/servlets/store_invite_servlet.py#L82-L91
	//      These can be easily retrieved by requesting the public rooms API
	//      server's database.

	requestURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/store-invite", body.IDServer)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Identity server %s responded with a %d error code", body.IDServer, resp.StatusCode)
		return nil, errors.New(errMsg)
	}

	var idResp idServerStoreInviteResponse
	err = json.NewDecoder(resp.Body).Decode(&idResp)
	return &idResp, err
}

// queryIDServerPubKey requests a public key identified with a given ID to the
// a given identity server and returns the matching base64-decoded public key.
// We assume that the ID server is trusted at this point.
// Returns an error if the request couldn't be sent, if its body couldn't be parsed
// or if the key couldn't be decoded from base64.
func queryIDServerPubKey(ctx context.Context, idServerName string, keyID string) ([]byte, error) {
	requestURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/pubkey/%s", idServerName, keyID)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	var pubKeyRes struct {
		PublicKey gomatrixserverlib.Base64Bytes `json:"public_key"`
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Couldn't retrieve key %s from server %s", keyID, idServerName)
		return nil, errors.New(errMsg)
	}

	err = json.NewDecoder(resp.Body).Decode(&pubKeyRes)
	return pubKeyRes.PublicKey, err
}

// checkIDServerSignatures iterates over the signatures of a requests.
// If no signature can be found for the ID server's domain, returns an error, else
// iterates over the signature for the said domain, retrieves the matching public
// key, and verify it.
// We assume that the ID server is trusted at this point.
// Returns nil if all the verifications succeeded.
// Returns an error if something failed in the process.
func checkIDServerSignatures(
	ctx context.Context, body *MembershipRequest, res *idServerLookupResponse,
) error {
	// Mashall the body so we can give it to VerifyJSON
	marshalledBody, err := json.Marshal(*res)
	if err != nil {
		return err
	}

	signatures, ok := res.Signatures[body.IDServer]
	if !ok {
		return errors.New("No signature for domain " + body.IDServer)
	}

	for keyID := range signatures {
		pubKey, err := queryIDServerPubKey(ctx, body.IDServer, keyID)
		if err != nil {
			return err
		}
		if err = gomatrixserverlib.VerifyJSON(body.IDServer, gomatrixserverlib.KeyID(keyID), pubKey, marshalledBody); err != nil {
			return err
		}
	}

	return nil
}

// emit3PIDInviteEvent builds and sends a "m.room.third_party_invite" event.
// Returns an error if something failed in the process.
func emit3PIDInviteEvent(
	ctx context.Context,
	body *MembershipRequest, res *idServerStoreInviteResponse,
	device *userapi.Device, roomID string, cfg *config.ClientAPI,
	rsAPI api.ClientRoomserverAPI,
	evTime time.Time,
) error {
	builder := &gomatrixserverlib.EventBuilder{
		Sender:   device.UserID,
		RoomID:   roomID,
		Type:     "m.room.third_party_invite",
		StateKey: &res.Token,
	}

	validityURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/pubkey/isvalid", body.IDServer)
	content := gomatrixserverlib.ThirdPartyInviteContent{
		DisplayName:    res.DisplayName,
		KeyValidityURL: validityURL,
		PublicKey:      res.PublicKey,
		PublicKeys:     res.PublicKeys,
	}

	if err := builder.SetContent(content); err != nil {
		return err
	}

	identity, err := cfg.Matrix.SigningIdentityFor(device.UserDomain())
	if err != nil {
		return err
	}

	queryRes := api.QueryLatestEventsAndStateResponse{}
	event, err := eventutil.QueryAndBuildEvent(ctx, builder, cfg.Matrix, identity, evTime, rsAPI, &queryRes)
	if err != nil {
		return err
	}

	return api.SendEvents(
		ctx, rsAPI,
		api.KindNew,
		[]*gomatrixserverlib.HeaderedEvent{
			event.Headered(queryRes.RoomVersion),
		},
		device.UserDomain(),
		cfg.Matrix.ServerName,
		cfg.Matrix.ServerName,
		nil,
		false,
	)
}
