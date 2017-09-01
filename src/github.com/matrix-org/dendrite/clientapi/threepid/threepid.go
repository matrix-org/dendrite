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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// EmailAssociationRequest represents the request defined at https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-register-email-requesttoken
type EmailAssociationRequest struct {
	IDServer    string `json:"id_server"`
	Secret      string `json:"client_secret"`
	Email       string `json:"email"`
	SendAttempt int    `json:"send_attempt"`
}

// EmailAssociationCheckRequest represents the request defined at https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-account-3pid
type EmailAssociationCheckRequest struct {
	Creds Credentials `json:"threePidCreds"`
	Bind  bool        `json:"bind"`
}

// Credentials represents the "ThreePidCredentials" structure defined at https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-account-3pid
type Credentials struct {
	SID      string `json:"sid"`
	IDServer string `json:"id_server"`
	Secret   string `json:"client_secret"`
}

// CreateSession creates a session on an identity server.
// Returns the session's ID.
// Returns an error if there was a problem sending the request or decoding the
// response, or if the identity server responded with a non-OK status.
func CreateSession(req EmailAssociationRequest) (string, error) {
	// TODO: Check if the ID server is trusted

	// Create a session on the ID server
	postURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/validate/email/requestToken", req.IDServer)

	data := url.Values{}
	data.Add("client_secret", req.Secret)
	data.Add("email", req.Email)
	data.Add("send_attempt", strconv.Itoa(req.SendAttempt))

	request, err := http.NewRequest("POST", postURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}

	// Error if the status isn't OK
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Could not create a session on the server %s", req.IDServer)
	}

	// Extract the SID from the response and return it
	var sid struct {
		SID string `json:"sid"`
	}
	err = json.NewDecoder(resp.Body).Decode(&sid)

	return sid.SID, err
}

// CheckAssociation checks the status of an ongoing association validation on an
// identity server.
// Returns a boolean set to true if the association has been validated, false if not.
// If the association has been validated, also returns the related third-party
// identifier and its medium.
// Returns an error if there was a problem sending the request or decoding the
// response, or if the identity server responded with a non-OK status.
func CheckAssociation(creds Credentials) (bool, string, string, error) {
	// TODO: Check if the ID server is trusted
	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/3pid/getValidated3pid?sid=%s&client_secret=%s", creds.IDServer, creds.SID, creds.Secret)
	resp, err := http.Get(url)
	if err != nil {
		return false, "", "", err
	}

	var respBody struct {
		Medium      string `json:"medium"`
		ValidatedAt int64  `json:"validated_at"`
		Address     string `json:"address"`
		ErrCode     string `json:"errcode"`
		Error       string `json:"error"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return false, "", "", err
	}

	if respBody.ErrCode == "M_SESSION_NOT_VALIDATED" {
		return false, "", "", nil
	} else if len(respBody.ErrCode) > 0 {
		return false, "", "", errors.New(respBody.Error)
	}

	return true, respBody.Address, respBody.Medium, nil
}

// PublishAssociation publishes a validated association between a third-party
// identifier and a Matrix ID.
// Returns an error if there was a problem sending the request or decoding the
// response, or if the identity server responded with a non-OK status.
func PublishAssociation(creds Credentials, userID string) error {
	// TODO: Check if the ID server is trusted
	postURL := fmt.Sprintf("https://%s/_matrix/identity/api/v1/3pid/bind", creds.IDServer)

	data := url.Values{}
	data.Add("sid", creds.SID)
	data.Add("client_secret", creds.Secret)
	data.Add("mxid", userID)

	request, err := http.NewRequest("POST", postURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	// Error if the status isn't OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Could not publish the association on the server %s", creds.IDServer)
	}

	return nil
}
