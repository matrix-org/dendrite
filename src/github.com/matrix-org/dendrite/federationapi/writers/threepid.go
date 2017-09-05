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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/threepid"
	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type invite struct {
	MXID   string                `json:"mxid"`
	RoomID string                `json:"room_id"`
	Sender string                `json:"sender"`
	Token  string                `json:"token"`
	Signed common.TPInviteSigned `json:"signed"`
}

type invites struct {
	Medium  string   `json:"medium"`
	Address string   `json:"address"`
	MXID    string   `json:"mxid"`
	Invites []invite `json:"invites"`
}

// CreateInvitesFrom3PIDInvites implements POST /_matrix/federation/v1/3pid/onbind
func CreateInvitesFrom3PIDInvites(
	req *http.Request, queryAPI api.RoomserverQueryAPI, cfg config.Dendrite,
	producer *producers.RoomserverProducer,
) util.JSONResponse {
	var body invites
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	evs := []gomatrixserverlib.Event{}
	for _, inv := range body.Invites {
		err, event := createInviteFrom3PIDInvite(req, queryAPI, cfg, inv)
		if err != nil {
			return *err
		}
		if event != nil {
			evs = append(evs, *event)
		}
	}

	// Send all the events
	if err := producer.SendEvents(evs, cfg.Matrix.ServerName); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

func createInviteFrom3PIDInvite(
	req *http.Request, queryAPI api.RoomserverQueryAPI, cfg config.Dendrite,
	inv invite,
) (*util.JSONResponse, *gomatrixserverlib.Event) {
	// Check if the token was provided
	if inv.Signed.Token == "" {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown("Rejecting received notification of third-party invite without signed"),
		}, nil
	}

	// Check the signatures
	marshalledSigned, err := json.Marshal(inv.Signed)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}
	if err := threepid.CheckIDServerSignatures("", inv.Signed.Signatures, marshalledSigned); err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}

	// Build the event
	builder := &gomatrixserverlib.EventBuilder{
		Type:     "m.room.member",
		Sender:   inv.Sender,
		RoomID:   inv.RoomID,
		StateKey: &inv.MXID,
	}

	content := common.MemberContent{
		// TODO: Load the profile
		Membership: "invite",
		ThirdPartyInvite: common.TPInvite{
			Signed: inv.Signed,
		},
	}

	if err := builder.SetContent(content); err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}

	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	if err = queryAPI.QueryLatestEventsAndState(&queryReq, &queryRes); err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}

	if !queryRes.RoomExists {
		// TODO: Use federation to auth the event
		return nil, nil
	}

	if err = fillDisplayName(builder, content, queryRes.StateEvents); err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}

	// Finish building the event
	builder.Depth = queryRes.Depth
	builder.PrevEvents = queryRes.LatestEvents

	authEvents := gomatrixserverlib.NewAuthEvents(nil)

	for i := range queryRes.StateEvents {
		authEvents.AddEvent(&queryRes.StateEvents[i])
	}

	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}
	builder.AuthEvents = refs

	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), cfg.Matrix.ServerName)
	now := time.Now()
	event, err := builder.Build(eventID, now, cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey)
	if err != nil {
		resErr := httputil.LogThenError(req, err)
		return &resErr, nil
	}

	return nil, &event
}

func fillDisplayName(
	builder *gomatrixserverlib.EventBuilder, content common.MemberContent,
	stateEvents []gomatrixserverlib.Event,
) error {
	// Look for the m.room.third_party_invite event
	var thirdPartyInviteEvent gomatrixserverlib.Event
	for _, event := range stateEvents {
		if event.Type() == "m.room.third_party_invite" {
			thirdPartyInviteEvent = event
		}
	}

	var thirdPartyInviteContent common.ThirdPartyInviteContent
	if err := json.Unmarshal(thirdPartyInviteEvent.Content(), &thirdPartyInviteContent); err != nil {
		return err
	}

	// Use the m.room.third_party_invite event to fill the "displayname" and
	// update the m.room.member event's content with it
	content.ThirdPartyInvite.DisplayName = thirdPartyInviteContent.DisplayName
	if err := builder.SetContent(content); err != nil {
		return err
	}

	return nil
}
