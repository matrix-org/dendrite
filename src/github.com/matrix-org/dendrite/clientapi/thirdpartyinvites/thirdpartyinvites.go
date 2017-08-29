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

package thirdpartyinvites

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/events"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/util"
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
	PublicKey   string             `json:"public_key"`
	Token       string             `json:"token"`
	DisplayName string             `json:"display_name"`
	PublicKeys  []common.PublicKey `json:"public_keys"`
}

// CheckAndProcess analyses the body of an incoming membership request.
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
func CheckAndProcess(
	req *http.Request, device *authtypes.Device, body *MembershipRequest,
	cfg config.Dendrite, queryAPI api.RoomserverQueryAPI, db *accounts.Database,
	producer *producers.RoomserverProducer, membership string, roomID string,
) *util.JSONResponse {
	if membership != "invite" || (body.Address == "" && body.IDServer == "" && body.Medium == "") {
		// If none of the 3PID-specific fields are supplied, it's a standard invite
		// so return nil for it to be processed as such
		return nil
	} else if body.Address == "" || body.IDServer == "" || body.Medium == "" {
		// If at least one of the 3PID-specific fields is supplied but not all
		// of them, return an error
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'address', 'id_server' and 'medium' must all be supplied"),
		}
	}

	lookupRes, storeInviteRes, err := queryIDServer(req, db, cfg, device, body, roomID)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr
	}

	if lookupRes.MXID == "" {
		// No Matrix ID could be found for this 3PID, meaning that a
		// "m.room.third_party_invite" have to be emitted from the data in
		// storeInviteRes.
		err = emit3PIDInviteEvent(body, storeInviteRes, device, roomID, cfg, queryAPI, producer)
		if err == events.ErrRoomNoExists {
			return &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(err.Error()),
			}
		} else if err != nil {
			resErr := httputil.LogThenError(req, err)
			return &resErr
		}

		// If everything went well, returns with an empty response.
		return &util.JSONResponse{
			Code: 200,
			JSON: struct{}{},
		}
	}

	// A Matrix ID have been found: set it in the body request and let the process
	// continue to create a "m.room.member" event with an "invite" membership
	body.UserID = lookupRes.MXID

	return nil
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
	req *http.Request, db *accounts.Database, cfg config.Dendrite,
	device *authtypes.Device, body *MembershipRequest, roomID string,
) (lookupRes *idServerLookupResponse, storeInviteRes *idServerStoreInviteResponse, err error) {
	// Lookup the 3PID
	lookupRes, err = queryIDServerLookup(body)
	if err != nil {
		return
	}

	if lookupRes.MXID == "" {
		// No Matrix ID matches with the given 3PID, ask the server to store the
		// invite and return a token
		storeInviteRes, err = queryIDServerStoreInvite(db, cfg, device, body, roomID)
		return
	}

	// A Matrix ID matches with the given 3PID
	// Get timestamp in milliseconds to compare it with the timestamps provided
	// by the identity server
	now := time.Now().UnixNano() / 1000000
	if lookupRes.NotBefore > now || now > lookupRes.NotAfter {
		// If the current timestamp isn't in the time frame in which the association
		// is known to be valid, re-run the query
		return queryIDServer(req, db, cfg, device, body, roomID)
	}

	// Check the request signatures and send an error if one isn't valid
	if err = checkIDServerSignatures(body, lookupRes); err != nil {
		return
	}

	return
}

// queryIDServerLookup sends a response to the identity server on /_matrix/identity/api/v1/lookup
// and returns the response as a structure.
// Returns an error if the request failed to send or if the response couldn't be parsed.
func queryIDServerLookup(body *MembershipRequest) (*idServerLookupResponse, error) {
	address := url.QueryEscape(body.Address)
	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/lookup?medium=%s&address=%s", body.IDServer, body.Medium, address)
	resp, err := http.Get(url)
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
	db *accounts.Database, cfg config.Dendrite, device *authtypes.Device,
	body *MembershipRequest, roomID string,
) (*idServerStoreInviteResponse, error) {
	// Retrieve the sender's profile to get their display name
	localpart, serverName, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return nil, err
	}

	var profile *authtypes.Profile
	if serverName == cfg.Matrix.ServerName {
		profile, err = db.GetProfileByLocalpart(localpart)
		if err != nil {
			return nil, err
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

	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/store-invite", body.IDServer)
	req, err := http.NewRequest("POST", url, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		// TODO: Log the error supplied with the identity server?
		errMsg := fmt.Sprintf("Identity server %s responded with a %d error code", body.IDServer, resp.StatusCode)
		return nil, errors.New(errMsg)
	}

	var idResp idServerStoreInviteResponse
	err = json.NewDecoder(resp.Body).Decode(&idResp)
	return &idResp, err
}

// queryIDServerPubKey requests a public key identified with a given ID to the
// a given identity server and returns the matching base64-decoded public key.
// Returns an error if the request couldn't be sent, if its body couldn't be parsed
// or if the key couldn't be decoded from base64.
func queryIDServerPubKey(idServerName string, keyID string) (publicKey []byte, err error) {
	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/pubkey/%s", idServerName, keyID)
	resp, err := http.Get(url)
	if err != nil {
		return
	}

	var pubKeyRes struct {
		PublicKey string `json:"public_key"`
	}

	if resp.StatusCode != http.StatusOK {
		// TODO: Log the error supplied with the identity server?
		errMsg := fmt.Sprintf("Couldn't retrieve key %s from server %s", keyID, idServerName)
		return nil, errors.New(errMsg)
	}

	if err = json.NewDecoder(resp.Body).Decode(&pubKeyRes); err != nil {
		return nil, err
	}

	return base64.RawStdEncoding.DecodeString(pubKeyRes.PublicKey)
}

// checkIDServerSignatures iterates over the signatures of a requests.
// If no signature can be found for the ID server's domain, returns an error, else
// iterates over the signature for the said domain, retrieves the matching public
// key, and verify it.
// Returns nil if all the verifications succeeded.
// Returns an error if something failed in the process.
func checkIDServerSignatures(body *MembershipRequest, res *idServerLookupResponse) error {
	// Mashall the body so we can give it to VerifyJSON
	marshalledBody, err := json.Marshal(*res)
	if err != nil {
		return err
	}

	// TODO: Check if the domain is part of a list of trusted ID servers
	signatures, ok := res.Signatures[body.IDServer]
	if !ok {
		return errors.New("No signature for domain " + body.IDServer)
	}

	for keyID := range signatures {
		pubKey, err := queryIDServerPubKey(body.IDServer, keyID)
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
	body *MembershipRequest, res *idServerStoreInviteResponse,
	device *authtypes.Device, roomID string, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI, producer *producers.RoomserverProducer,
) error {
	builder := &gomatrixserverlib.EventBuilder{
		Sender:   device.UserID,
		RoomID:   roomID,
		Type:     "m.room.third_party_invite",
		StateKey: &res.Token,
	}

	validityURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/pubkey/isvalid", body.IDServer)
	content := common.ThirdPartyInviteContent{
		DisplayName:    res.DisplayName,
		KeyValidityURL: validityURL,
		PublicKey:      res.PublicKey,
	}

	content.PublicKeys = make([]common.PublicKey, len(res.PublicKeys))
	copy(content.PublicKeys, res.PublicKeys)

	if err := builder.SetContent(content); err != nil {
		return err
	}

	var queryRes *api.QueryLatestEventsAndStateResponse
	event, err := events.BuildEvent(builder, cfg, queryAPI, queryRes)
	if err != nil {
		return err
	}

	if err := producer.SendEvents([]gomatrixserverlib.Event{*event}, cfg.Matrix.ServerName); err != nil {
		return err
	}

	return nil
}
