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
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// The data used to populate the /consent request
type constentTemplateData struct {
	UserID       string
	Version      string
	UserHMAC     string
	HasConsented bool
	ReadOnly     bool
}

func writeHeaderAndText(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(http.StatusText(statusCode)))
}

func consent(writer http.ResponseWriter, req *http.Request, userAPI userapi.UserConsentPolicyAPI, cfg *config.ClientAPI) {
	consentCfg := cfg.Matrix.UserConsentOptions

	// The data used to populate the /consent request
	data := constentTemplateData{
		UserID:   req.FormValue("u"),
		Version:  req.FormValue("v"),
		UserHMAC: req.FormValue("h"),
	}

	switch req.Method {
	case http.MethodGet:
		// display the privacy policy without a form
		data.ReadOnly = data.UserID == "" || data.UserHMAC == "" || data.Version == ""

		// let's see if the user already consented to the current version
		if !data.ReadOnly {
			if ok, err := validHMAC(data.UserID, data.UserHMAC, consentCfg.FormSecret); err != nil || !ok {
				writeHeaderAndText(writer, http.StatusForbidden)
				return
			}

			res := &userapi.QueryPolicyVersionResponse{}
			localpart, _, err := gomatrixserverlib.SplitID('@', data.UserID)
			if err != nil {
				logrus.WithError(err).Error("unable to split username")
				writeHeaderAndText(writer, http.StatusInternalServerError)
				return
			}
			if err = userAPI.QueryPolicyVersion(req.Context(), &userapi.QueryPolicyVersionRequest{
				Localpart: localpart,
			}, res); err != nil {
				logrus.WithError(err).Error("unable query policy version")
				writeHeaderAndText(writer, http.StatusInternalServerError)
				return
			}
			data.HasConsented = res.PolicyVersion == consentCfg.Version
		}

		err := consentCfg.Templates.ExecuteTemplate(writer, consentCfg.Version+".gohtml", data)
		if err != nil {
			logrus.WithError(err).Error("unable to execute consent template")
			writeHeaderAndText(writer, http.StatusInternalServerError)
			return
		}
	case http.MethodPost:
		ok, err := validHMAC(data.UserID, data.UserHMAC, consentCfg.FormSecret)
		if err != nil || !ok {
			if !ok {
				writeHeaderAndText(writer, http.StatusForbidden)
				return
			}
			writeHeaderAndText(writer, http.StatusInternalServerError)
			return
		}
		localpart, _, err := gomatrixserverlib.SplitID('@', data.UserID)
		if err != nil {
			logrus.WithError(err).Error("unable to split username")
			writeHeaderAndText(writer, http.StatusInternalServerError)
			return
		}
		if err = userAPI.PerformUpdatePolicyVersion(
			req.Context(),
			&userapi.UpdatePolicyVersionRequest{
				PolicyVersion: data.Version,
				Localpart:     localpart,
			},
			&userapi.UpdatePolicyVersionResponse{},
		); err != nil {
			writeHeaderAndText(writer, http.StatusInternalServerError)
			return
		}
		// display the privacy policy without a form
		data.ReadOnly = false
		data.HasConsented = true

		err = consentCfg.Templates.ExecuteTemplate(writer, consentCfg.Version+".gohtml", data)
		if err != nil {
			logrus.WithError(err).Error("unable to print consent template")
			writeHeaderAndText(writer, http.StatusInternalServerError)
		}
	}
}

func sendServerNoticeForConsent(userAPI userapi.ClientUserAPI, rsAPI api.ClientRoomserverAPI,
	cfgNotices *config.ServerNotices,
	cfgClient *config.ClientAPI,
	senderDevice *userapi.Device,
	asAPI appserviceAPI.AppServiceQueryAPI,
) {
	res := &userapi.QueryOutdatedPolicyResponse{}
	if err := userAPI.QueryOutdatedPolicy(context.Background(), &userapi.QueryOutdatedPolicyRequest{
		PolicyVersion: cfgClient.Matrix.UserConsentOptions.Version,
	}, res); err != nil {
		logrus.WithError(err).Error("unable to fetch users with outdated consent policy")
		return
	}

	var (
		consentOpts  = cfgClient.Matrix.UserConsentOptions
		data         = make(map[string]string)
		err          error
		sentMessages int
	)

	if len(res.UserLocalparts) == 0 {
		return
	}

	logrus.WithField("count", len(res.UserLocalparts)).Infof("Sending server notice to users who have not yet accepted the policy")

	for _, localpart := range res.UserLocalparts {
		if localpart == cfgClient.Matrix.ServerNotices.LocalPart {
			continue
		}
		userID := fmt.Sprintf("@%s:%s", localpart, cfgClient.Matrix.ServerName)
		data["ConsentURL"], err = consentOpts.ConsentURL(userID)
		if err != nil {
			logrus.WithError(err).WithField("userID", userID).Error("unable to construct consentURI")
			continue
		}
		msgBody := &bytes.Buffer{}

		if err = consentOpts.TextTemplates.ExecuteTemplate(msgBody, "serverNoticeTemplate", data); err != nil {
			logrus.WithError(err).WithField("userID", userID).Error("unable to execute serverNoticeTemplate")
			continue
		}

		req := sendServerNoticeRequest{
			UserID: userID,
			Content: struct {
				MsgType string `json:"msgtype,omitempty"`
				Body    string `json:"body,omitempty"`
			}{
				MsgType: consentOpts.ServerNoticeContent.MsgType,
				Body:    msgBody.String(),
			},
		}
		_, err = sendServerNotice(context.Background(), req, rsAPI, cfgNotices, cfgClient, senderDevice, asAPI, userAPI, nil, nil, nil)
		if err != nil {
			logrus.WithError(err).WithField("userID", userID).Error("failed to send server notice for consent to user")
			continue
		}
		sentMessages++
		res := &userapi.UpdatePolicyVersionResponse{}
		if err = userAPI.PerformUpdatePolicyVersion(context.Background(), &userapi.UpdatePolicyVersionRequest{
			PolicyVersion:      consentOpts.Version,
			Localpart:          userID,
			ServerNoticeUpdate: true,
		}, res); err != nil {
			logrus.WithError(err).WithField("userID", userID).Error("failed to update policy version")
			continue
		}
	}
	if sentMessages > 0 {
		logrus.Infof("Sent messages to %d users", sentMessages)
	}
}

func validHMAC(username, userHMAC, secret string) (bool, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(username))
	if err != nil {
		return false, err
	}
	expectedMAC := mac.Sum(nil)
	decoded, err := hex.DecodeString(userHMAC)
	if err != nil {
		return false, err
	}
	return hmac.Equal(decoded, expectedMAC), nil
}
