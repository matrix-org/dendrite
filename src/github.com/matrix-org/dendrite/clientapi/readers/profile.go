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

package readers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"

	sarama "gopkg.in/Shopify/sarama.v1"
)

type profileResponse struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"displayname"`
}

type avatarURL struct {
	AvatarURL string `json:"avatar_url"`
}

type displayName struct {
	DisplayName string `json:"displayname"`
}

// TODO: Move this struct to `common` so the components that consume the topic
// can use it when parsing incoming messages
type profileUpdate struct {
	Updated  string `json:"updated"`   // Which attribute is updated (can be either `avatar_url` or `displayname`)
	OldValue string `json:"old_value"` // The attribute's value before the update
	NewValue string `json:"new_value"` // The attribute's value after the update
}

// GetProfile implements GET /profile/{userID}
func GetProfile(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	if req.Method != "GET" {
		return util.JSONResponse{
			Code: 405,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}
	localpart := getLocalPart(userID)
	profile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	res := profileResponse{
		AvatarURL:   profile.AvatarURL,
		DisplayName: profile.DisplayName,
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// GetAvatarURL implements GET /profile/{userID}/avatar_url
func GetAvatarURL(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	localpart := getLocalPart(userID)
	profile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	res := avatarURL{
		AvatarURL: profile.AvatarURL,
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// SetAvatarURL implements PUT /profile/{userID}/avatar_url
func SetAvatarURL(
	req *http.Request, accountDB *accounts.Database, userID string,
	cfg config.Dendrite,
) util.JSONResponse {
	var r avatarURL
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}
	if r.AvatarURL == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'avatar_url' must be supplied."),
		}
	}

	localpart := getLocalPart(userID)

	oldProfile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := accountDB.SetAvatarURL(localpart, r.AvatarURL); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := sendUpdate(userID, oldProfile, r.AvatarURL, "", cfg); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// GetDisplayName implements GET /profile/{userID}/displayname
func GetDisplayName(
	req *http.Request, accountDB *accounts.Database, userID string,
) util.JSONResponse {
	localpart := getLocalPart(userID)
	profile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	res := displayName{
		DisplayName: profile.DisplayName,
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// SetDisplayName implements PUT /profile/{userID}/displayname
func SetDisplayName(
	req *http.Request, accountDB *accounts.Database, userID string,
	cfg config.Dendrite,
) util.JSONResponse {
	var r displayName
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}
	if r.DisplayName == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'displayname' must be supplied."),
		}
	}

	localpart := getLocalPart(userID)

	oldProfile, err := accountDB.GetProfileByLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := accountDB.SetDisplayName(localpart, r.DisplayName); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := sendUpdate(userID, oldProfile, "", r.DisplayName, cfg); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

func getLocalPart(userID string) string {
	if !strings.HasPrefix(userID, "@") {
		panic(fmt.Errorf("Invalid user ID"))
	}

	// Get the part before ":"
	username := strings.Split(userID, ":")[0]
	// Return the part after the "@"
	return strings.Split(username, "@")[1]
}

// Send an update using kafka to notify the roomserver of the profile update
// Returns an error if the update failed to send
func sendUpdate(userID string, oldProfile *authtypes.Profile, newAvatarURL string,
	newDisplayName string, cfg config.Dendrite,
) error {
	var update profileUpdate
	var m sarama.ProducerMessage

	m.Topic = string(cfg.Kafka.Topics.UserUpdates)
	m.Key = sarama.StringEncoder(userID)

	// Determining if the changed value is the avatar URL or the display name
	if len(newAvatarURL) > 0 {
		update = profileUpdate{
			Updated:  "avatar_url",
			OldValue: oldProfile.AvatarURL,
			NewValue: newAvatarURL,
		}
	} else if len(newDisplayName) > 0 {
		update = profileUpdate{
			Updated:  "displayname",
			OldValue: oldProfile.DisplayName,
			NewValue: newDisplayName,
		}
	} else {
		panic(fmt.Errorf("No update to send"))
	}

	value, err := json.Marshal(update)
	if err != nil {
		return err
	}
	m.Value = sarama.ByteEncoder(value)

	producer, err := sarama.NewSyncProducer(cfg.Kafka.Addresses, nil)
	if err != nil {
		return err
	}

	if _, _, err := producer.SendMessage(&m); err != nil {
		return err
	}

	return nil
}
