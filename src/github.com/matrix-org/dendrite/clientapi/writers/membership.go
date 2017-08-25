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

package writers

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

type membershipRequestBody struct {
	UserID   string `json:"user_id"`
	Reason   string `json:"reason"`
	IDServer string `json:"id_server"`
	Medium   string `json:"medium"`
	Address  string `json:"address"`
}

// SendMembership implements PUT /rooms/{roomID}/(join|kick|ban|unban|leave|invite)
// by building a m.room.member event then sending it to the room server
func SendMembership(
	req *http.Request, accountDB *accounts.Database, device *authtypes.Device,
	roomID string, membership string, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI, producer *producers.RoomserverProducer,
) util.JSONResponse {
	var body membershipRequestBody
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	if res := checkAndProcess3PIDInvite(req, device, &body, cfg, queryAPI, producer, roomID); res != nil {
		return *res
	}

	stateKey, reason, reqErr := getMembershipStateKey(body, device, membership)
	if reqErr != nil {
		return *reqErr
	}

	profile, err := loadProfile(stateKey, cfg, accountDB)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	builder := gomatrixserverlib.EventBuilder{
		Sender:   device.UserID,
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &stateKey,
	}

	// "unban" or "kick" isn't a valid membership value, change it to "leave"
	if membership == "unban" || membership == "kick" {
		membership = "leave"
	}

	content := common.MemberContent{
		Membership:  membership,
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL,
		Reason:      reason,
	}

	if err = builder.SetContent(content); err != nil {
		return httputil.LogThenError(req, err)
	}

	event, err := events.BuildEvent(&builder, cfg, queryAPI, nil)
	if err == events.ErrRoomNoExists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound(err.Error()),
		}
	} else if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := producer.SendEvents([]gomatrixserverlib.Event{*event}, cfg.Matrix.ServerName); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

func loadProfile(userID string, cfg config.Dendrite, accountDB *accounts.Database) (*authtypes.Profile, error) {
	localpart, serverName, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	var profile *authtypes.Profile
	if serverName == cfg.Matrix.ServerName {
		profile, err = accountDB.GetProfileByLocalpart(localpart)
	} else {
		profile = &authtypes.Profile{}
	}

	return profile, err
}

// getMembershipStateKey extracts the target user ID of a membership change.
// For "join" and "leave" this will be the ID of the user making the change.
// For "ban", "unban", "kick" and "invite" the target user ID will be in the JSON request body.
// In the latter case, if there was an issue retrieving the user ID from the request body,
// returns a JSONResponse with a corresponding error code and message.
func getMembershipStateKey(
	body membershipRequestBody, device *authtypes.Device, membership string,
) (stateKey string, reason string, response *util.JSONResponse) {
	if membership == "ban" || membership == "unban" || membership == "kick" || membership == "invite" {
		// If we're in this case, the state key is contained in the request body,
		// possibly along with a reason (for "kick" and "ban") so we need to parse
		// it
		if body.UserID == "" {
			response = &util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON("'user_id' must be supplied."),
			}
			return
		}

		stateKey = body.UserID
		reason = body.Reason
	} else {
		stateKey = device.UserID
	}

	return
}

func checkAndProcess3PIDInvite(
	req *http.Request, device *authtypes.Device, body *membershipRequestBody,
	cfg config.Dendrite, queryAPI api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer, roomID string,
) *util.JSONResponse {
	if body.Address == "" && body.IDServer == "" && body.Medium == "" {
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

	resp, err := queryIDServer(req, device, body, roomID)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr
	}

	if resp.Lookup.MXID == "" {
		event, err := make3PIDInviteEvent(body, resp.StoreInvite, device, roomID, cfg, queryAPI)
		if err == events.ErrRoomNoExists {
			return &util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound(err.Error()),
			}
		} else if err != nil {
			resErr := httputil.LogThenError(req, err)
			return &resErr
		}

		if err := producer.SendEvents([]gomatrixserverlib.Event{*event}, cfg.Matrix.ServerName); err != nil {
			resErr := httputil.LogThenError(req, err)
			return &resErr
		}

		return &util.JSONResponse{
			Code: 200,
			JSON: struct{}{},
		}
	}

	// Set the Matrix user ID from the body request and let the process
	// continue to create a "m.room.member" event
	body.UserID = resp.Lookup.MXID

	return nil
}

type idServerResponses struct {
	Lookup      *idServerLookupResponse
	StoreInvite *idServerStoreInviteResponse
}

type idServerLookupResponse struct {
	TS         int64                        `json:"ts"`
	NotBefore  int64                        `json:"not_before"`
	NotAfter   int64                        `json:"not_after"`
	Medium     string                       `json:"medium"`
	Address    string                       `json:"address"`
	MXID       string                       `json:"mxid"`
	Signatures map[string]map[string]string `json:"signatures"`
}

type idServerStoreInviteResponse struct {
	PublicKey   string             `json:"public_key"`
	Token       string             `json:"token"`
	DisplayName string             `json:"display_name"`
	PublicKeys  []common.PublicKey `json:"public_keys"`
}

func queryIDServer(
	req *http.Request, device *authtypes.Device, body *membershipRequestBody,
	roomID string,
) (res *idServerResponses, err error) {
	res = new(idServerResponses)
	res.Lookup, err = queryIDServerLookup(body)
	if err != nil {
		return
	}
	if res.Lookup.MXID == "" {
		res.StoreInvite, err = queryIDServerStoreInvite(device, body, roomID)
		return
	}

	// Get timestamp in milliseconds to compare it
	now := time.Now().UnixNano() / 1000000
	if res.Lookup.NotBefore > now || now > res.Lookup.NotAfter {
		// If the current timestamp isn't in the time frame in which the association
		// is known to be valid, re-run the query
		return queryIDServer(req, device, body, roomID)
	}

	ok, err := checkIDServerSignatures(body, res.Lookup)
	if err != nil {
		return
	}
	if !ok {
		err = errors.New("The identity server's identity could not be verified")
		return
	}

	return
}

func queryIDServerLookup(body *membershipRequestBody) (res *idServerLookupResponse, err error) {
	address := url.QueryEscape(body.Address)
	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/lookup?medium=%s&address=%s", body.IDServer, body.Medium, address)
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	// TODO: Check status code
	res = new(idServerLookupResponse)
	err = json.NewDecoder(resp.Body).Decode(res)
	return
}

func queryIDServerStoreInvite(device *authtypes.Device, body *membershipRequestBody, roomID string) (*idServerStoreInviteResponse, error) {
	client := http.Client{}

	data := url.Values{}
	data.Add("medium", body.Medium)
	data.Add("address", body.Address)
	data.Add("room_id", roomID)
	data.Add("sender", device.UserID)

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
		errMsg := fmt.Sprintf("Identity server %s responded with a %d error code", body.IDServer, resp.StatusCode)
		return nil, errors.New(errMsg)
	}

	idResp := new(idServerStoreInviteResponse)
	err = json.NewDecoder(resp.Body).Decode(idResp)
	return idResp, err
}

func queryIDServerPubKey(body *membershipRequestBody, keyID string) (publicKey []byte, err error) {
	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/pubkey/%s", body.IDServer, keyID)
	resp, err := http.Get(url)
	if err != nil {
		return
	}

	var pubKeyRes struct {
		PublicKey string `json:"public_key"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&pubKeyRes); err != nil {
		return nil, err
	}
	// TODO: Store the public key in the database and, if there's one stored, retrieve
	// it and verify its validity (/isvalid) instead of fetching it
	return base64.RawStdEncoding.DecodeString(pubKeyRes.PublicKey)
}

func checkIDServerSignatures(body *membershipRequestBody, res *idServerLookupResponse) (ok bool, err error) {
	marshalledBody, err := json.Marshal(*res)
	if err != nil {
		return
	}

	for domain, signatures := range res.Signatures {
		for keyID := range signatures {
			pubKey, err := queryIDServerPubKey(body, keyID)
			if err != nil {
				return false, err
			}
			if err = gomatrixserverlib.VerifyJSON(domain, gomatrixserverlib.KeyID(keyID), pubKey, marshalledBody); err != nil {
				return false, nil
			}
		}
	}

	return true, nil
}

func make3PIDInviteEvent(
	body *membershipRequestBody, res *idServerStoreInviteResponse,
	device *authtypes.Device, roomID string, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
) (*gomatrixserverlib.Event, error) {
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
		return nil, err
	}

	var queryRes *api.QueryLatestEventsAndStateResponse
	return events.BuildEvent(builder, cfg, queryAPI, queryRes)
}
