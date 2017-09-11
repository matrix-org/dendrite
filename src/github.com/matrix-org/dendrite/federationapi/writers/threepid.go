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
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
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
	producer *producers.RoomserverProducer, federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	var body invites
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	evs := []gomatrixserverlib.Event{}
	for _, inv := range body.Invites {
		event, err := createInviteFrom3PIDInvite(queryAPI, cfg, inv, federation)
		if err != nil {
			return httputil.LogThenError(req, err)
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

// createInviteFrom3PIDInvite processes an invite provided by the identity server
// and creates a m.room.member event (with "invite" membership) from it.
// Returns an error if there was a problem building the event or fetching the
// necessary data to do so.
func createInviteFrom3PIDInvite(
	queryAPI api.RoomserverQueryAPI, cfg config.Dendrite, inv invite,
	federation *gomatrixserverlib.FederationClient,
) (*gomatrixserverlib.Event, error) {
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
		ThirdPartyInvite: &common.TPInvite{
			Signed: inv.Signed,
		},
	}

	if err := builder.SetContent(content); err != nil {
		return nil, err
	}

	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, err
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	if err = queryAPI.QueryLatestEventsAndState(&queryReq, &queryRes); err != nil {
		return nil, err
	}

	if !queryRes.RoomExists {
		// Use federation to auth the event
		var remoteServer gomatrixserverlib.ServerName
		_, remoteServer, err = gomatrixserverlib.SplitID('!', inv.RoomID)
		if err != nil {
			return nil, err
		}
		err = federation.ExchangeThirdPartyInvite(remoteServer, *builder)
		return nil, err
	} else {
		// Auth the event locally
		builder.Depth = queryRes.Depth
		builder.PrevEvents = queryRes.LatestEvents

		authEvents := gomatrixserverlib.NewAuthEvents(nil)

		for i := range queryRes.StateEvents {
			authEvents.AddEvent(&queryRes.StateEvents[i])
		}

		if err = fillDisplayName(builder, content, authEvents); err != nil {
			return nil, err
		}

		refs, err := eventsNeeded.AuthEventReferences(&authEvents)
		if err != nil {
			return nil, err
		}
		builder.AuthEvents = refs
	}

	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), cfg.Matrix.ServerName)
	now := time.Now()
	event, err := builder.Build(eventID, now, cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

// fillDisplayName looks in a list of auth events for a m.room.third_party_invite
// event with the state key matching a given m.room.member event's content's token.
// If such an event is found, fills the "display_name" attribute of the
// "third_party_invite" structure in the m.room.member event with the display_name
// from the m.room.third_party_invite event.
// Returns an error if there was a problem parsing the m.room.third_party_invite
// event's content or updating the m.room.member event's content.
// Returns nil if no m.room.third_party_invite with a matching token could be
// found. Returning an error isn't necessary in this case as the event will be
// rejected by gomatrixserverlib.
func fillDisplayName(
	builder *gomatrixserverlib.EventBuilder, content common.MemberContent,
	authEvents gomatrixserverlib.AuthEvents,
) error {
	// Look for the m.room.third_party_invite event
	thirdPartyInviteEvent, _ := authEvents.ThirdPartyInvite(content.ThirdPartyInvite.Signed.Token)

	if thirdPartyInviteEvent == nil {
		// If the third party invite event doesn't exist then we can't use it to set the display name.
		return nil
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
