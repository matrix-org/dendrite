// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package routing

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type presenceReq struct {
	Presence  string  `json:"presence"`
	StatusMsg *string `json:"status_msg,omitempty"`
}

// set a unix timestamp of when it last saw the types
// this way it can filter based on time
var lastPresence map[int]int64 = make(map[int]int64)

// how long before the online status expires
// should be long enough that any client will have another sync before expiring
const presenceTimeout int64 = 10

func SetPresence(
	req *http.Request,
	cfg *config.ClientAPI,
	device *api.Device,
	producer *producers.SyncAPIProducer,
	userID string,
) util.JSONResponse {

	//grab time for caching
	workingTime := time.Now().Unix()

	if !cfg.Matrix.Presence.EnableOutbound {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Unable to set presence for other user."),
		}
	}
	var presence presenceReq
	parseErr := httputil.UnmarshalJSONRequest(req, &presence)
	if parseErr != nil {
		return *parseErr
	}

	presenceStatus, ok := types.PresenceFromString(presence.Presence)
	if !ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown(fmt.Sprintf("Unknown presence '%s'.", presence.Presence)),
		}
	}

	//update time for each presence
	lastPresence[int(presenceStatus)] = workingTime

	//online will always get priority
	if (workingTime - lastPresence[int(types.PresenceOnline)]) < presenceTimeout {
		err := producer.SendPresence(req.Context(), userID, types.PresenceOnline, presence.StatusMsg)
		if err != nil {
			log.WithError(err).Errorf("failed to update presence to Online")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		//idle gets secondary priority because your presence shouldnt be idle if you are on a different device
		//kinda copying discord presence
	} else if (workingTime - lastPresence[int(types.PresenceUnavailable)]) < presenceTimeout {
		err := producer.SendPresence(req.Context(), userID, types.PresenceUnavailable, presence.StatusMsg)
		if err != nil {
			log.WithError(err).Errorf("failed to update presence to Unavailable (~idle)")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		//only set offline status if there is no known online devices
		//clients may set offline to attempt to not alter the online status of the user
	} else if (workingTime - lastPresence[int(types.PresenceOffline)]) < presenceTimeout {
		err := producer.SendPresence(req.Context(), userID, types.PresenceOffline, presence.StatusMsg)
		if err != nil {
			log.WithError(err).Errorf("failed to update presence to Offline")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		//set unknown if there is truly no devices that we know the state of
	} else {
		err := producer.SendPresence(req.Context(), userID, types.PresenceUnknown, presence.StatusMsg)
		if err != nil {
			log.WithError(err).Errorf("failed to update presence as Unknown")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func GetPresence(
	req *http.Request,
	device *api.Device,
	natsClient *nats.Conn,
	presenceTopic string,
	userID string,
) util.JSONResponse {
	msg := nats.NewMsg(presenceTopic)
	msg.Header.Set(jetstream.UserID, userID)

	presence, err := natsClient.RequestMsg(msg, time.Second*10)
	if err != nil {
		log.WithError(err).Errorf("unable to get presence")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	statusMsg := presence.Header.Get("status_msg")
	e := presence.Header.Get("error")
	if e != "" {
		log.Errorf("received error msg from nats: %s", e)
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: types.PresenceClientResponse{
				Presence: types.PresenceUnavailable.String(),
			},
		}
	}
	lastActive, err := strconv.Atoi(presence.Header.Get("last_active_ts"))
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	p := types.PresenceInternal{LastActiveTS: spec.Timestamp(lastActive)}
	currentlyActive := p.CurrentlyActive()
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: types.PresenceClientResponse{
			CurrentlyActive: &currentlyActive,
			LastActiveAgo:   p.LastActiveAgo(),
			Presence:        presence.Header.Get("presence"),
			StatusMsg:       &statusMsg,
		},
	}
}
